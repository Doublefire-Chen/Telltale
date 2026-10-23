# Telltale

[English](README.md) · **简体中文**

一个轻量、自托管的 VPS 监控。每台机器上运行一个小巧的 **Agent**，把 CPU、内存、
磁盘、交换、负载、网络流量与带宽、运行时间和系统信息上报给 **Dashboard**；
Dashboard 实时展示这些数据并保存历史。Telltale（指示器）只显示状态、从不改变任何东西，
这正是它的核心理念：

> **Telltale 只观察，从不动手。**

Agent 只往外发送数据，对收到的任何回应都不采取行动。没有 WebSSH、没有远程执行命令、
没有文件管理、没有远程配置、也没有自动更新——不是可选项，而是永远不会有。
Dashboard 一侧的任何东西都无法在你的机器上运行代码、修改配置或升级程序，
上游的一次发布也永远不会自己把你的整批机器弄坏。原因见
[这里](docs/adr/0001-agent-is-one-way.md)。

## 功能

- **实时状态页**：每台主机一张卡片，显示国旗和系统图标、CPU/内存/交换/磁盘使用率条、
  带宽、流量、负载、运行时间以及进程数和连接数，随上报实时更新（默认每 2 秒一次）；
  连续错过三个上报周期即显示离线。
- **历史图表**：1 小时、24 小时、7 天和 30 天的图表，以原始上报、1 分钟平均和
  15 分钟平均三档保存在 SQLite 中。
- 每台主机的 **IPv4 和 IPv6 地址**及其国家/地区，由 Agent 自己连接 Dashboard 得知——
  不查询任何第三方服务。
- **默认私有**；也可以公开状态页，但公开页面永远不显示 IP 地址和已隐藏的主机。
- **升级永远不会弄坏一批机器**：旧 Agent 能继续配合新 Dashboard 工作，反之亦然
  （见[协议](docs/protocol.md)）。
- **轻量**：Agent 是一个无需 root 的静态二进制文件，内存约 10–15 MB，
  CPU 占用不到单核的百分之一。
- 界面支持 **English 和简体中文**。
- Agent 支持 **Linux、Windows 和 macOS**（amd64 与 arm64）；Dashboard 运行在 Linux 上。

## 安装

每个 [Release](https://github.com/Doublefire-Chen/Telltale/releases) 都附带二进制文件：
`telltale-agent-<系统>-<架构>`（Linux、Windows、macOS；amd64 与 arm64）和
`telltale-dashboard-linux-<架构>`，全部由同一个标签、以同一个版本号构建。
若要从源码构建，需要 Go 和 Node.js：

```sh
make geoip   # 下载要内嵌的 DB-IP Lite 国家数据库（可选）
make build   # 生成 bin/telltale-dashboard 和 bin/telltale-agent
```

### 校验下载的文件

`SHA256SUMS` 列出了每个二进制文件的校验和；`SHA256SUMS.sigstore.json` 是
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/) 签名，
证明该文件确实由本仓库的发布流程在对应标签上生成。签名绑定的是发布流程的 GitHub 身份，
没有可被盗取的签名密钥。把 `VERSION` 设为对应版本，例如 `VERSION=v0.1.0`：

```sh
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity "https://github.com/Doublefire-Chen/Telltale/.github/workflows/release.yml@refs/tags/$VERSION"
sha256sum --ignore-missing -c SHA256SUMS   # macOS：shasum -a 256 --ignore-missing -c SHA256SUMS
```

`install.sh --version` 总会用 `SHA256SUMS` 校验二进制文件；如果还想确认发布者，
请自己运行上面的 `cosign` 命令。

### 1. Dashboard（Linux）

```sh
sudo install -m 0755 telltale-dashboard /usr/local/bin/
sudo useradd --system --user-group --no-create-home --shell /usr/sbin/nologin telltale
sudo install -D -m 0644 deploy/dashboard.toml /etc/telltale/dashboard.toml
sudo install -m 0644 deploy/systemd/telltale-dashboard.service /etc/systemd/system/
sudo systemctl enable --now telltale-dashboard

# 创建管理员账号（用户名为 "admin"）。没有网页版的初始化向导。
sudo -u telltale telltale-dashboard admin set-password
```

Dashboard 监听 `127.0.0.1:8080` 和 `[::1]:8080`。请在前面放一个反向代理来提供 HTTPS：
[`deploy/nginx/telltale.conf`](deploy/nginx/telltale.conf) 是现成的 nginx 配置
（HTTP/2、IPv4 与 IPv6、WebSocket）。

**请为 Dashboard 的域名同时添加 A 和 AAAA 记录。** 每个 Agent 会分别通过 IPv4 和 IPv6
各上报一次，Dashboard 正是借此得知主机的两个地址。只有 IPv4 的 Dashboard 只能显示 IPv4 地址。

### 2. 添加主机

登录后，在 **添加主机** 中输入名称（留空则使用机器的 hostname），然后复制页面显示的配置。
其中的主机令牌**只显示这一次**，Dashboard 只保存它的哈希值。之后可以重新生成令牌，
旧令牌会立即失效。

### 3. Agent

**Linux** —— 请先阅读 [`deploy/install.sh`](deploy/install.sh)：它很短，
并写明了会动哪些文件。

```sh
sudo sh install.sh install --dashboard-url https://status.example.com --token tt_… --version vX.Y.Z
# 或者使用你手上已有的二进制文件：--binary ./telltale-agent
```

它会创建无特权的 `telltale-agent` 用户，写入只有该用户能读取的配置文件，
并启动一个加固过的 systemd 服务。升级时只带 `--version` 或 `--binary` 再运行一次即可。
完全卸载：`sudo sh install.sh uninstall`。

**Windows**（以管理员身份）—— 把 `telltale-agent.exe` 放到 `C:\Program Files\Telltale\`，
把配置保存为 `C:\ProgramData\Telltale\agent.toml`，然后：

```powershell
& "C:\Program Files\Telltale\telltale-agent.exe" service install
```

它以 LocalService 身份运行，日志在事件查看器中。

**macOS** —— 把 `telltale-agent` 放到 `/usr/local/bin/`，把配置保存为
`/etc/telltale/agent.toml`，然后：

```sh
sudo telltale-agent service install
```

它以隐藏的 `_telltale-agent` 用户运行，日志写入 `/var/log/telltale-agent.log`。
用 `service uninstall` 卸载（加 `--purge` 会同时删除配置文件）。

安装程序永远不会修改你的 motd、shell 配置文件、`authorized_keys`、crontab、sysctl、
防火墙、DNS 或软件源。

### 升级

Dashboard 和 Agent 可以分别升级，顺序随意。比 Dashboard 旧的 Agent 会显示
"Agent 版本过旧" 标记，但仍能正常工作。Agent 永远不会自己更新。

## 文档

- [配置参考](docs/configuration.md)
- [Agent 与 Dashboard 之间的协议及其兼容性规则](docs/protocol.md)
- [术语表](CONTEXT.md)：本项目的术语——Agent、Dashboard、Host、Report、Host Info、
  Traffic、Bandwidth、Offline……
- [架构决策记录](docs/adr/)，包括 Telltale 永远不会做的事

## 参与贡献

请先阅读 [`CONTEXT.md`](CONTEXT.md) 和 [`docs/adr/`](docs/adr/)。特别是：
让 Dashboard 能对主机采取行动的功能——远程 shell 或命令、文件访问、远程配置、
自动更新——永远不在范围之内，而不只是暂时不做。

```sh
make test   # Go 与前端测试
./scripts/test-install-in-container.sh   # 安装脚本与 systemd 服务（需要 Docker）
./scripts/test-nginx-in-container.sh     # 示例 nginx 配置（需要 Docker）
go test -tags budget ./internal/e2e      # Agent 的 CPU、内存和体积预算（Linux，约 1 分钟）
```

发布时推送一个标签（例如 `v0.1.0`）即可：发布流程会自动测试、构建、签名并发布全部文件。

## 许可证

GPL-3.0。IP 地理位置数据由 [DB-IP](https://db-ip.com) 提供（CC BY 4.0）。
国旗图标来自 [flag-icons](https://github.com/lipis/flag-icons)（MIT），
系统图标来自 [font-logos](https://github.com/Lukas-W/font-logos)（公有领域）。
