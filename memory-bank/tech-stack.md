## 技术栈 (Tech Stack)
- **语言与运行时**：Go 1.25，充分利用 goroutine 与 context 传递。模块化管理。
- **框架与库**：
  - `github.com/go-telegram/bot` 负责 Telegram Bot API 交互。
  - `go.mongodb.org/mongo-driver` 作为 MongoDB 驱动。
  - `github.com/sirupsen/logrus` 提供结构化日志能力。
- **基础设施**：MongoDB 作为持久化存储；Docker Compose（`docker-compose.local.yml`）用于本地开发与集成。
- **配置管理**：`internal/config` 统一加载环境变量，涵盖 Telegram Token、Bot Owner、Mongo URI 及四方支付访问密钥。