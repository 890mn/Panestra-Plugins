# Panestra Plugins

[Panestra](https://github.com/890mn/Panestra) 的第一方插件仓库，包含软件适配、插件清单、IPC SDK、测试与独立签名打包工具

插件下载到 Windows Core 后在本机运行，Android 只负责显示和发送已授权操作。运行时不依赖 GitHub，主程序不编译本仓库的适配源码

## 插件

| 插件            | 功能                                                    |
| --------------- | ------------------------------------------------------- |
| System Monitor  | 处理器、内存、磁盘、网络与主机信息，授权后锁定会话      |
| Codex           | 复用本机 ChatGPT 登录，读取订阅额度、窗口与重置卡       |
| GLM Coding Plan | 智谱中国区个人编程套餐用量，需要 API Key                |
| DeepSeek        | 人民币与美元 API 余额、赠送和充值构成，需要 API Key     |
| 网易云音乐      | 歌曲、封面、播放、切歌与进度控制，控制需单独授权        |
| Clash Verge     | HTTP 或命名管道读取 Mihomo 状态，授权后切换模式和节点   |
| ALAS            | Azur Lane AutoScript 实例、当前任务与调度队列，只读桥接 |

Codex 当前提供订阅用量，不执行任务。GLM 查询 Coding Plan 用量，DeepSeek 查询 API 账户余额。网易云的进度通道需要播放器提供系统时间轴，或通过回环地址连接本机调试端口；内部接口不兼容时禁用跳转。Clash 的自动测速组保持只读，系统代理与 TUN 由 Clash Verge 管理。ALAS 生成手动启动桥接，不自动重启正在运行的脚本

## 网易云音乐进度

没有系统时间轴的播放器版本需要本机进度通道。在插件设置中保存端口（例如 `19228`），点击「修复启动方式」，退出网易云并从原来的快捷方式重新打开

修复只给桌面、开始菜单、任务栏快捷方式和已有用户自启动项补充回环地址进度参数，保留其他参数，不自动重启播放器。修改前保存 DPAPI 加密备份；「恢复启动设置」还原已修改的参数，并保留用户后续的手动改动。升级播放器后若快捷方式被覆盖，可再次检查启动入口

公共目录不可写时创建对应的用户快捷方式，不需要管理员权限。恢复会移除新增入口，公共快捷方式保持原样

直接运行 EXE 或其他启动器需要携带 `--remote-debugging-address=127.0.0.1 --remote-debugging-port=19228`。此方式依赖播放器的本机接口，接口不兼容时不估算进度，保留系统播放控制并禁用跳转

## 构建与检查

需要 Windows x64、Go 1.27+ 和 PowerShell。ALAS 桥接测试还需 Python 3.7+，仅使用标准库

```powershell
git clone https://github.com/890mn/Panestra-Plugins.git
cd Panestra-Plugins
go mod download
.\scripts\build.ps1
.\scripts\verify.ps1
```

可用 `-Plugins codex` 只构建一个插件。可执行文件和对应清单写入 `artifacts/插件名`，各插件版本由各自的 manifest 决定，并写入握手信息

指定便携 Go 时设置 `PANESTRA_GO` 为 go.exe 的完整路径，GOPATH 与 GOCACHE 可自行设置，工具链不进入仓库

## 签名与独立发布

每个插件包包含 `manifest.json` 和该插件的 EXE，脚本、桥接和读取实现已嵌入对应可执行文件。包不包含设备数据、账户凭据或私钥

1. 构建插件和发布工具

```powershell
.\scripts\build.ps1
go build -trimpath -o artifacts/publisher.exe ./cmd/publisher
```

2. 首次发行使用自己的密钥，放在仓库外的安全目录

```powershell
.\artifacts\publisher.exe --generate-key --key 'C:\Publisher\plugins.protected'
```

私钥由当前 Windows 用户的 DPAPI 保护，旁边的 `.pub` 是可公开的信任根。自定义发行版需要将公钥同步到 Panestra 主仓库的 `core/plugins/publisher.pub` 后重新构建；已有客户端只接受其内置受信发布者，不能仅替换下载包中的公钥绕过校验

3. 打包并指定递增序号

```powershell
.\scripts\package.ps1 -Key 'C:\Publisher\plugins.protected' -Sequence 1
```

输出目录 `artifacts/packages/1` 中包含目录清单和各插件的 ZIP、JSON、SIG。序号按插件递增，已安装版本拒绝回放和降级；相同输出不会覆盖。后续版本先修改对应 manifest 的版本号，再构建并使用更大的序号

主程序可通过「导入插件包」选择三个文件安装，或在构建时用 `-PluginPackages` 附带离线包。GitHub Release 上传 `catalog.json` 与目录引用的 ZIP、JSON、SIG，即可供主程序手动检查和下载；发布脚本只产生本地文件，不推送或发布

## 添加插件

在 `plugins/名称` 编写实现与 manifest，在 `cmd/名称/main.go` 调用 SDK，再将插件加入 `catalog.json`。主程序按已安装清单生成设置、组件目录和订阅，不需要新增软件适配分支

现有插件 API 支持数值、网络、主机、额度、账户、代理、媒体和任务等通用视图，兼容旧布局的 renderer 标识保持稳定。每类组件至少完整适配 2×2、4×2、4×3、4×4，支持按尺寸保存内部预设与可选表现。新增数据源可复用这些视图，新增基础渲染类型需要先更新宿主协议

原生插件是受信代码，进程隔离、角色检查和资源限制不等同于操作系统文件或网络沙箱。所有读取结果必须去除凭据，配置保存在 Core 提供的插件专属数据目录，不监听公共服务，不把私钥写入日志

详细约定见 [PLUGIN_API.md](PLUGIN_API.md)

## 目录

| 目录      | 内容                                 |
| --------- | ------------------------------------ |
| `plugins` | 适配实现、资源、清单与单元测试       |
| `cmd`     | 各插件入口与签名打包工具             |
| `sdk`     | 本机 IPC、角色检查、DPAPI 与会话操作 |
| `scripts` | 独立构建、检查和打包                 |
| `tests`   | ALAS 只读桥接合同测试                |

## 更新说明

### 网易云音乐 0.1.1

新增启动入口修复、恢复与状态说明，修复普通快捷方式重新启动后丢失播放进度的问题，修改前保存本机加密备份

### 0.1.0

从 Panestra 主仓库拆分七项适配，提供独立进程、清单配置、版本握手与签名包，保留原有的状态读取、播放控制、凭据保护和 ALAS 桥接
