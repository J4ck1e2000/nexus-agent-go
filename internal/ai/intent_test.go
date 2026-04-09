package ai

import "testing"

func TestIntentClassifier_ParseTopK(t *testing.T) {
	classifier := NewIntentClassifier()

	zh := classifier.Classify("推荐两台适合启动训练任务的机器", nil)
	if zh.Type != IntentScheduleSuggestion {
		t.Fatalf("zh intent type mismatch: got=%q want=%q", zh.Type, IntentScheduleSuggestion)
	}
	if zh.TopK != 2 {
		t.Fatalf("zh topK mismatch: got=%d want=2", zh.TopK)
	}

	enWord := classifier.Classify("recommend two nodes for training", nil)
	if enWord.TopK != 2 {
		t.Fatalf("en(word) topK mismatch: got=%d want=2", enWord.TopK)
	}

	enNumber := classifier.Classify("recommend 2 nodes for training", nil)
	if enNumber.TopK != 2 {
		t.Fatalf("en(number) topK mismatch: got=%d want=2", enNumber.TopK)
	}
}
