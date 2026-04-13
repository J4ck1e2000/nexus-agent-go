# nexus-agent-go

项目环境配置

## CMD 版本

```cmd
set AI_ENABLED=true
set AI_MODE=agent
set AI_PROVIDER=qwen
set AI_MODEL=qwen3.5-122b-a10b
set AI_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
set AI_API_KEY=your-api-key

set "MYSQL_DSN=root:lcy030529..@tcp(127.0.0.1:3306)/nexus_agent?charset=utf8mb4&parseTime=True&loc=Local"
set "JWT_SECRET=your-jwt-secret"

go run .\cmd\gateway\
```

## PowerShell 版本

```powershell
$env:AI_ENABLED="true"
$env:AI_MODE="agent"
$env:AI_PROVIDER="qwen"
$env:AI_MODEL="qwen3.5-122b-a10b"
$env:AI_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"
$env:AI_API_KEY="your-api-key"

$env:MYSQL_DSN="root:lcy030529..@tcp(127.0.0.1:3306)/nexus_agent?charset=utf8mb4&parseTime=True&loc=Local"
$env:JWT_SECRET="your-jwt-secret"

go run .\cmd\gateway\
```
