package ai

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	defaultChunkTargetRunes = 360
	defaultChunkMinRunes    = 180
	defaultChunkMaxRunes    = 520
)

// KnowledgeBase holds loaded documents and chunk indexable units.
type KnowledgeBase struct {
	Documents []KnowledgeDocument
	Chunks    []KnowledgeChunk
}

// LoadKnowledgeBase loads local markdown files and builds chunked knowledge units.
func LoadKnowledgeBase(knowledgeDir string) (KnowledgeBase, error) {
	dir := strings.TrimSpace(knowledgeDir)
	if dir == "" {
		return KnowledgeBase{}, fmt.Errorf("knowledge dir is empty")
	}

	info, err := os.Stat(dir)
	if err != nil {
		return KnowledgeBase{}, err
	}
	if !info.IsDir() {
		return KnowledgeBase{}, fmt.Errorf("knowledge path is not a directory: %s", dir)
	}

	files, err := collectMarkdownFiles(dir)
	if err != nil {
		return KnowledgeBase{}, err
	}

	base := KnowledgeBase{
		Documents: make([]KnowledgeDocument, 0, len(files)),
		Chunks:    make([]KnowledgeChunk, 0, len(files)*4),
	}

	for _, path := range files {
		document, err := parseKnowledgeDocument(path)
		if err != nil {
			return KnowledgeBase{}, err
		}
		base.Documents = append(base.Documents, document)
		base.Chunks = append(base.Chunks, splitDocumentChunks(document)...)
	}
	return base, nil
}

func collectMarkdownFiles(knowledgeDir string) ([]string, error) {
	files := make([]string, 0, 16)
	err := filepath.WalkDir(knowledgeDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d == nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		files = append(files, filepath.Clean(path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func parseKnowledgeDocument(path string) (KnowledgeDocument, error) {
	rawBytes, err := os.ReadFile(path)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	raw := strings.TrimSpace(strings.TrimPrefix(string(rawBytes), "\uFEFF"))
	if raw == "" {
		return KnowledgeDocument{}, fmt.Errorf("knowledge document is empty: %s", path)
	}

	title, category, tags := extractDocumentMetadata(raw)
	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if title == "" {
		title = strings.ReplaceAll(baseName, "_", " ")
	}
	if category == "" {
		category = "general"
	}

	return KnowledgeDocument{
		ID:         sanitizeIdentifier(baseName),
		Title:      title,
		Category:   strings.ToLower(strings.TrimSpace(category)),
		Tags:       normalizeTags(tags),
		SourcePath: filepath.ToSlash(path),
		RawContent: raw,
	}, nil
}

func extractDocumentMetadata(raw string) (title string, category string, tags []string) {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
		if trimmed == "" {
			continue
		}
		if title == "" && strings.HasPrefix(trimmed, "#") {
			title = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "category:") {
			category = strings.TrimSpace(trimmed[len("Category:"):])
			continue
		}
		if strings.HasPrefix(lower, "tags:") {
			tagLine := strings.TrimSpace(trimmed[len("Tags:"):])
			tags = splitTags(tagLine)
		}
	}
	return title, category, tags
}

func splitTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	normalized := strings.NewReplacer("，", ",", "；", ",", ";", ",", "|", ",").Replace(raw)
	parts := strings.Split(normalized, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		result = append(result, strings.ToLower(tag))
	}
	return normalizeTags(result)
}

func splitDocumentChunks(doc KnowledgeDocument) []KnowledgeChunk {
	sections := splitMarkdownSections(doc.RawContent, doc.Title)
	if len(sections) == 0 {
		return nil
	}

	chunks := make([]KnowledgeChunk, 0, len(sections)*2)
	seq := 1
	for _, section := range sections {
		contentParts := splitSectionContent(section.Content, defaultChunkTargetRunes, defaultChunkMinRunes, defaultChunkMaxRunes)
		for _, content := range contentParts {
			chunkContent := strings.TrimSpace(content)
			if chunkContent == "" {
				continue
			}
			chunks = append(chunks, KnowledgeChunk{
				ID:          fmt.Sprintf("%s:%03d", doc.ID, seq),
				DocumentID:  doc.ID,
				Title:       doc.Title,
				Category:    doc.Category,
				Tags:        append([]string(nil), doc.Tags...),
				Content:     chunkContent,
				HeadingPath: section.HeadingPath,
				SourcePath:  doc.SourcePath,
			})
			seq++
		}
	}
	return chunks
}

type markdownSection struct {
	HeadingPath string
	Content     string
}

func splitMarkdownSections(raw, title string) []markdownSection {
	lines := strings.Split(raw, "\n")
	sections := make([]markdownSection, 0, 8)
	headings := make([]string, 0, 4)
	if strings.TrimSpace(title) != "" {
		headings = append(headings, strings.TrimSpace(title))
	}
	currentHeadingPath := joinHeadingPath(headings)

	buffer := make([]string, 0, 16)
	flush := func() {
		merged := cleanChunkText(strings.Join(buffer, "\n"))
		buffer = buffer[:0]
		if merged == "" {
			return
		}
		sections = append(sections, markdownSection{
			HeadingPath: currentHeadingPath,
			Content:     merged,
		})
	}

	for _, line := range lines {
		level, heading, isHeading := parseMarkdownHeading(line)
		if !isHeading {
			buffer = append(buffer, line)
			continue
		}

		flush()
		if level <= 0 {
			level = 1
		}
		if level > 6 {
			level = 6
		}

		if len(headings) == 0 {
			headings = append(headings, strings.TrimSpace(title))
		}

		targetLen := level
		if targetLen < 1 {
			targetLen = 1
		}
		if targetLen > len(headings) {
			targetLen = len(headings)
		}
		headings = headings[:targetLen]

		if trimmed := strings.TrimSpace(heading); trimmed != "" {
			headings = append(headings, trimmed)
		}
		currentHeadingPath = joinHeadingPath(headings)
	}
	flush()

	if len(sections) == 0 {
		cleaned := cleanChunkText(raw)
		if cleaned != "" {
			sections = append(sections, markdownSection{
				HeadingPath: currentHeadingPath,
				Content:     cleaned,
			})
		}
	}
	return sections
}

func parseMarkdownHeading(line string) (level int, heading string, ok bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	for _, r := range trimmed {
		if r == '#' {
			level++
			continue
		}
		break
	}
	if level == 0 {
		return 0, "", false
	}
	heading = strings.TrimSpace(trimmed[level:])
	return level, heading, true
}

func joinHeadingPath(headings []string) string {
	if len(headings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(headings))
	for _, heading := range headings {
		trimmed := strings.TrimSpace(heading)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " / ")
}

func splitSectionContent(content string, targetRunes, minRunes, maxRunes int) []string {
	paragraphs := splitParagraphs(content)
	if len(paragraphs) == 0 {
		return nil
	}

	chunks := make([]string, 0, len(paragraphs))
	current := make([]string, 0, 4)
	currentRunes := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.TrimSpace(strings.Join(current, "\n\n")))
		current = current[:0]
		currentRunes = 0
	}

	for _, para := range paragraphs {
		paraRunes := utf8.RuneCountInString(para)
		if paraRunes > maxRunes {
			if len(current) > 0 {
				flush()
			}
			chunks = append(chunks, splitLongText(para, maxRunes)...)
			continue
		}

		if currentRunes >= minRunes && currentRunes+paraRunes > targetRunes {
			flush()
		}
		current = append(current, para)
		currentRunes += paraRunes
	}
	flush()
	return chunks
}

func splitParagraphs(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	blocks := strings.Split(normalized, "\n\n")
	result := make([]string, 0, len(blocks))
	for _, block := range blocks {
		cleaned := cleanChunkText(block)
		if cleaned == "" {
			continue
		}
		result = append(result, cleaned)
	}
	return result
}

func splitLongText(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = defaultChunkMaxRunes
	}
	trimmed := cleanChunkText(text)
	if trimmed == "" {
		return nil
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return []string{trimmed}
	}

	result := make([]string, 0, len(runes)/maxRunes+1)
	start := 0
	for start < len(runes) {
		end := start + maxRunes
		if end >= len(runes) {
			result = append(result, strings.TrimSpace(string(runes[start:])))
			break
		}

		split := end
		for cursor := end; cursor > start+maxRunes/2; cursor-- {
			if isChunkBreakRune(runes[cursor-1]) {
				split = cursor
				break
			}
		}
		result = append(result, strings.TrimSpace(string(runes[start:split])))
		start = split
	}
	return result
}

func isChunkBreakRune(r rune) bool {
	switch r {
	case '.', ',', ';', ':', '!', '?', '\n', '。', '，', '；', '：', '！', '？':
		return true
	default:
		return false
	}
}

func cleanChunkText(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "category:") || strings.HasPrefix(lower, "tags:") {
			continue
		}
		if trimmed == "" {
			if len(filtered) > 0 && filtered[len(filtered)-1] == "" {
				continue
			}
			filtered = append(filtered, "")
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func sanitizeIdentifier(raw string) string {
	input := strings.ToLower(strings.TrimSpace(raw))
	if input == "" {
		return "doc"
	}

	var builder strings.Builder
	underscore := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			underscore = false
			continue
		}
		if !underscore {
			builder.WriteByte('_')
			underscore = true
		}
	}

	normalized := strings.Trim(builder.String(), "_")
	if normalized == "" {
		return "doc"
	}
	return normalized
}
