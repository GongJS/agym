# agym

简体中文 | [English](./README.md)

面向 macOS 的轻量级 `agy` 多账户管理工具：提供菜单栏托盘弹窗与命令行 CLI（单二进制分发），支持安全切换钥匙串登录态、查询 Gemini/Claude 配额窗口与模型目录。

基于 Go 与 [Wails v3](https://wails.io/) 构建，前端采用零构建依赖的原生 HTML/CSS/JavaScript。

## 核心特性

- **单二进制双模式**：直接运行 `agym`（或 `agym tray`）启动 macOS 菜单栏托盘应用；带子命令运行时作为 CLI 工具使用。
- **安全的钥匙串切换**：通过 macOS Keychain（`gemini-agy-backup/<email>`）备份多账号登录态，配合跨进程 `slot.lock` 文件锁安全切换；原始凭据不会传入前端或日志。
- **官方 OAuth 登录与自动拉取额度**：支持在托盘内直接拉起官方 `agy` OAuth 登录新账户，添加账户成功后自动查询配额。
- **配额与模型展示**：展示 Gemini / Claude 5 小时与每周额度、订阅等级、出口地区，以及单账户模型与思考等级下拉偏好。
- **中英文双语界面**：默认跟随系统语言，支持右上角 `EN` / `中文` 一键切换。
- **会话额度接力（CLI）**：提供 `supervise` 与 Stop Hook，在外部终端中遇到严格配额耗尽事件时安全轮换账户。

## 环境要求

- **macOS**（依赖 macOS 钥匙串与原生 WebView）
- 已安装官方 `agy` CLI（默认路径 `~/.local/bin/agy`）

## 安装

### 从源码编译

```sh
git clone https://github.com/gongjs/agym.git
cd agym
make build
cp dist/agym ~/.local/bin/agy-tools
```

### Homebrew（Tap）

发布到 Homebrew Tap 后可通过以下命令安装并设置自启：

```sh
brew tap gongjs/tap
brew install agym
brew services start agym
```

## 使用方法

### 启动菜单栏托盘

```sh
agym
# 或显式指定
agym tray
```

### 常用 CLI 命令

```sh
agym accounts                 # 以 JSON 输出账户元数据（不含凭据）
agym who                      # 查看当前活跃 agy 账户
agym switch <email>           # 切换到已保存的账户
agym quota [--refresh]        # 查看缓存额度（加 --refresh 立即刷新）
agym add <email>              # 将钥匙串已有备份加入管理名单
agym remove <email>           # 从名单移除（保留钥匙串备份）
agym rotation <email> on|off  # 开启/关闭自动轮换
agym install-hook             # 安装额度接力 Stop hook
agym remove-hook              # 移除 agym Stop hook
agym supervise [--pinned] [--] <agy args...>
```

## 相关文档

- [架构设计与安全边界](./docs/architecture.md)
- [本地开发与发布指南](./docs/development.md)
- [贡献指南](./CONTRIBUTING.md)
- [安全策略](./SECURITY.md)

## 开源协议

[MIT](./LICENSE)
