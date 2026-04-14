param(
    [ValidateSet("gateway", "sync", "reload", "sync-reload", "eval", "env")]
    [string]$Task = "gateway",

    [ValidateSet("auto", "local", "qdrant")]
    [string]$Backend = "auto",

    [string]$KnowledgeDir = "knowledge/anomalies",

    [int]$TopK = 5,

    [switch]$Recreate,

    [string]$GatewayURL = "http://127.0.0.1:3000",

    [string]$ReloadToken = "",

    [string]$Username = "",

    [string]$Password = ""
)

$ErrorActionPreference = "Stop"

function Set-EnvDefault {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Value
    )
    $current = [Environment]::GetEnvironmentVariable($Name, "Process")
    if ([string]::IsNullOrWhiteSpace($current)) {
        [Environment]::SetEnvironmentVariable($Name, $Value, "Process")
    }
}

function Set-EnvIfEmpty {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string]$Value = ""
    )
    $current = [Environment]::GetEnvironmentVariable($Name, "Process")
    if ([string]::IsNullOrWhiteSpace($current) -and -not [string]::IsNullOrWhiteSpace($Value)) {
        [Environment]::SetEnvironmentVariable($Name, $Value, "Process")
    }
}

function Mask-Secret {
    param([string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) { return "" }
    if ($Value.Length -le 6) { return "***" }
    return $Value.Substring(0, 3) + "***" + $Value.Substring($Value.Length - 2)
}

function Invoke-KnowledgeReload {
    param(
        [Parameter(Mandatory = $true)][string]$BaseURL,
        [string]$Token = "",
        [string]$User = "",
        [string]$Pass = ""
    )

    $tokenToUse = $Token
    if ([string]::IsNullOrWhiteSpace($tokenToUse)) {
        if ([string]::IsNullOrWhiteSpace($User) -or [string]::IsNullOrWhiteSpace($Pass)) {
            throw "reload requires -ReloadToken or -Username/-Password."
        }
        $loginUrl = ($BaseURL.TrimEnd("/") + "/api/login")
        $loginBody = @{ username = $User; password = $Pass } | ConvertTo-Json -Depth 4
        $loginResp = Invoke-RestMethod -Method Post -Uri $loginUrl -ContentType "application/json" -Body $loginBody
        $tokenToUse = $loginResp.token
        if ([string]::IsNullOrWhiteSpace($tokenToUse)) {
            throw "login succeeded but token is empty."
        }
    }

    $reloadUrl = ($BaseURL.TrimEnd("/") + "/api/ai/knowledge/reload")
    $headers = @{ Authorization = ("Bearer {0}" -f $tokenToUse) }
    $resp = Invoke-RestMethod -Method Post -Uri $reloadUrl -Headers $headers -ContentType "application/json" -Body "{}"
    Write-Host ("[knowledge-run] reload result: {0}" -f ($resp | ConvertTo-Json -Depth 5 -Compress))
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

Push-Location $repoRoot
try {
    Set-EnvDefault "AI_RAG_ENABLED" "true"
    Set-EnvDefault "AI_RAG_KNOWLEDGE_DIR" $KnowledgeDir

    Set-EnvDefault "KNOWLEDGE_RETRIEVAL_BACKEND" $Backend
    Set-EnvDefault "KNOWLEDGE_QDRANT_ENABLED" "true"
    Set-EnvDefault "KNOWLEDGE_QDRANT_HOST" "127.0.0.1"
    Set-EnvDefault "KNOWLEDGE_QDRANT_PORT" "6333"
    Set-EnvDefault "KNOWLEDGE_QDRANT_USE_TLS" "false"
    Set-EnvDefault "KNOWLEDGE_QDRANT_COLLECTION" "knowledge_chunks"
    Set-EnvDefault "KNOWLEDGE_QDRANT_TIMEOUT_SEC" "5"

    $aiProviderNow = [Environment]::GetEnvironmentVariable("AI_PROVIDER", "Process")
    $aiProviderLower = ""
    if (-not [string]::IsNullOrWhiteSpace($aiProviderNow)) {
        $aiProviderLower = $aiProviderNow.ToLowerInvariant().Trim()
    }
    $isAliProvider = @("qwen", "dashscope", "aliyun") -contains $aiProviderLower

    $defaultEmbeddingModel = "text-embedding-v4"
    $defaultEmbeddingBaseURL = "https://api.openai.com/v1"
    if ($isAliProvider) {
        $defaultEmbeddingModel = "text-embedding-v3"
        $defaultEmbeddingBaseURL = [Environment]::GetEnvironmentVariable("AI_BASE_URL", "Process")
        if ([string]::IsNullOrWhiteSpace($defaultEmbeddingBaseURL)) {
            $defaultEmbeddingBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
        }
    }

    Set-EnvDefault "KNOWLEDGE_EMBEDDING_ENABLED" "true"
    Set-EnvDefault "KNOWLEDGE_EMBEDDING_PROVIDER" "openai_compatible"
    Set-EnvDefault "KNOWLEDGE_EMBEDDING_MODEL" $defaultEmbeddingModel
    Set-EnvDefault "KNOWLEDGE_EMBEDDING_BASE_URL" $defaultEmbeddingBaseURL
    Set-EnvDefault "KNOWLEDGE_EMBEDDING_DIM" "1024"
    $aiApiKey = [Environment]::GetEnvironmentVariable("AI_API_KEY", "Process")
    Set-EnvIfEmpty "KNOWLEDGE_EMBEDDING_API_KEY" $aiApiKey

    Set-EnvDefault "KNOWLEDGE_TOPK" "3"
    Set-EnvDefault "KNOWLEDGE_MIN_SCORE" "0.35"
    $defaultSyncBatchSize = "32"
    if ($isAliProvider) {
        $defaultSyncBatchSize = "10"
    }
    Set-EnvDefault "KNOWLEDGE_SYNC_BATCH_SIZE" $defaultSyncBatchSize

    $backendNow = [Environment]::GetEnvironmentVariable("KNOWLEDGE_RETRIEVAL_BACKEND", "Process")
    $embedKeyNow = [Environment]::GetEnvironmentVariable("KNOWLEDGE_EMBEDDING_API_KEY", "Process")
    $qdrantKeyNow = [Environment]::GetEnvironmentVariable("KNOWLEDGE_QDRANT_API_KEY", "Process")

    Write-Host ("[knowledge-run] backend={0}, knowledge_dir={1}" -f $backendNow, $KnowledgeDir)
    if ($isAliProvider) {
        Write-Host ("[knowledge-run] detected aliyun provider: AI_PROVIDER={0}" -f $aiProviderLower)
    }
    Write-Host ("[knowledge-run] embedding_api_key={0}, qdrant_api_key={1}" -f (Mask-Secret $embedKeyNow), (Mask-Secret $qdrantKeyNow))

    if (($backendNow -ne "local") -and [string]::IsNullOrWhiteSpace($embedKeyNow)) {
        Write-Warning "KNOWLEDGE_EMBEDDING_API_KEY is empty. backend=qdrant/auto may fallback to local or fail."
    }

    switch ($Task) {
        "gateway" {
            & go run ./cmd/gateway
            break
        }
        "sync" {
            $args = @("./cmd/knowledge-sync", "--knowledge-dir", $KnowledgeDir)
            if ($Recreate) {
                $args += "--recreate"
            }
            & go run @args
            break
        }
        "eval" {
            $args = @("./cmd/rag-eval", ("--backend={0}" -f $backendNow), ("--topk={0}" -f $TopK), "--knowledge-dir", $KnowledgeDir)
            & go run @args
            break
        }
        "reload" {
            Invoke-KnowledgeReload -BaseURL $GatewayURL -Token $ReloadToken -User $Username -Pass $Password
            break
        }
        "sync-reload" {
            $args = @("./cmd/knowledge-sync", "--knowledge-dir", $KnowledgeDir)
            if ($Recreate) {
                $args += "--recreate"
            }
            & go run @args
            Invoke-KnowledgeReload -BaseURL $GatewayURL -Token $ReloadToken -User $Username -Pass $Password
            break
        }
        "env" {
            Write-Host "[knowledge-run] environment prepared only (no command executed)."
            break
        }
    }
}
finally {
    Pop-Location
}
