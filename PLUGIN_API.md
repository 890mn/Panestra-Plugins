# Plugin API v1

## 包与清单

Schema 2 使用全局插件 ID、三段版本、native-process 引擎和包内 EXE。`engine.pluginApi` 为 `>=1.0.0 <2.0.0`，`maxChildren` 为 0–4，清单不超过 64 KiB，不接受宿主 JavaScript

清单声明 `permissions`、`sources`、`widgets`、`routes` 和 `ui`。只允许声明的数据源发布状态，只允许声明的操作从设备调用。源 ID 使用字母、数字、点、下划线或短横线；轮询间隔 250–60000 ms，状态变化可即时发布

`ui.settings` 声明字段 key、label 与 text / secret / integer / boolean 类型。status 或 setup 指定返回值绑定，secret 字段不预填。可配置插件实现 configure、refresh、status；使用凭据的插件实现 clearCredential，并仅返回 hasCredential。配置由 Owner 操作，Viewer 和 Operator 的设置界面只读

`ui.ownerActions` 可提供清除连接等额外设置操作，包含 label、operation 与 JSON body，仅向 Owner 显示，operation 仍需在 routes 中声明

`routes` 声明 GET / POST、以 /integrations/ 开头的路径、operation 和最低角色。配置可用 grantField 与 grant 绑定读取权限；Role 从 Core 的认证结果产生，客户端不能自行指定。控制操作需检查 Operator / Owner 和插件自身的 allowControl

`widgets` 中 renderer 固定为 declarative，id 选择宿主支持的视图，subscriptions 引用已声明源。提供 defaults、catalog、sizePresets 和 presentations，至少适配 2×2、4×2、4×3、4×4，表现包含 auto 和至少一个可选模式

账户视图可在 defaults 中指定 valueMode: usage / balance。catalog 为同类组件提供不同标题、数据源和默认值。初始工作空间可由 initialWidgets 提供，只在空页面首次应用，不覆盖已有布局或删除记录

legacyData 和 legacyPaths 仅用于从旧 Core 迁移已存在的加密配置与桥接文件，使用包外的插件数据目录，不替换已有插件配置，也不删除旧文件

## IPC

stdin / stdout 为父进程持有的本机管道。每帧是 4 字节大端 JSON 长度，后接 UTF-8 JSON，最大 256 KiB，使用 SDK Framer 写入以避免并发交错。stdout 不打印日志，诊断写 stderr

SDK 先发送 hello：pluginId、version、protocolVersion=1、manifestDigest。Core 对照已验证包确认身份，再返回 grantedCapabilities。完成初始化后发送 health，随后响应宿主心跳，发布 publish 消息，格式为 source 与 value

request 包含 operation、role、body，返回 result 的 status 与 JSON body。SDK 每次最多处理 4 个请求，单次 30 秒超时，Core 验证角色、注册路由与响应格式。不要读取 HTTP Token、设备私钥或 Core 会话数据库

## 入口示例

```go
sdk.Run("dev.example.metrics", func(ctx context.Context, dir string, grants []string, publish sdk.Publish) (sdk.Handler, error) {
    // 在 dir 保存配置，先按 grants 决定是否启动读取
    return func(ctx context.Context, request sdk.Request) sdk.Response {
        switch request.Operation {
        case "status":
            return sdk.Result(map[string]any{"enabled": false, "state": "disabled"})
        case "configure":
            if !sdk.CanConfigure(request) { return sdk.Denied() }
            // 用 sdk.Decode 严格校验请求，保存配置后发布去除凭据的状态
        }
        return sdk.Failure(404, "NOT_FOUND", "未知插件操作")
    }, nil
})
```

## 生命周期

Core 先下载、验证 Ed25519 元数据签名、ZIP 摘要和文件摘要，再展示清单及权限。确认后启动候选进程，握手与健康检查通过才写入当前安装记录；失败保留原进程、安装序号和权限

插件拥有独立版本目录与数据目录。停用保留安装，卸载停止进程但保留数据和布局。宿主重启只验证并启动本机已安装包，不访问 GitHub；配置变化、进程故障和账户不可用应明确发布状态，不能用零值或伪造图表代替未知数据

私钥、安装包、账户数据和工具链不提交。测试使用临时目录、模拟账户或显式授权的真实环境，验证签名拒绝、权限隔离、秘密不进入遥测、版本失败保留与常见预设尺寸
