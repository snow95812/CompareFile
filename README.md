# CompareFile

基于 Go + HTML + CSS + JavaScript 重写的重复文件查找器，界面与核心功能参照 `../findFile` 目录下的 Electron 项目实现。

## 项目说明

参考项目 `findFile` 的原始形态是一个 Electron 桌面应用：

- 主进程负责窗口创建、目录选择、回收站删除、Finder 定位和 Quick Look 预览
- 渲染进程负责目录列表、扫描进度、重复文件分组、筛选和批量选择
- 扫描服务负责递归遍历目录、按文件大小与扩展名预筛、再按 SHA-256 做内容去重

当前项目保留了这套产品形态，但把运行时替换成了：

- Go 负责桌面能力、静态资源服务、文件扫描和系统调用
- HTML / CSS / JavaScript 保留原界面结构与交互方式
- macOS 输出 `.app`
- Windows 11 输出 `.exe`

## 当前保留的能力

- 多目录选择
- 目录列表去重、移除与清空
- 按文件大小和扩展名预筛选候选文件
- 基于 SHA-256 的内容校验
- 扫描进度展示
- 扫描中止
- 重复文件分组、分组折叠、全选、反选、自动选择
- 文件类型筛选：视频、音频、图片
- 文件大小筛选：`0-1M`、`1-2M`、`2-3M`、`3-4M`、`4-5M`、`5-10M`、`10M以上`
- 图片缩略图展示
- 文件预览
- 打开所在文件夹
- 删除到回收站 / 废纸篓
- 删除中止
- 删除进度展示（百分比 + 已处理数量 / 总数量）
- 缩略图缓存清理
- 未处理项列表展示

## 界面结构

界面布局直接参照参考项目首页结构：

- 左侧边栏
- 选择目录 / 开始扫描操作区
- 已选目录列表
- 扫描说明
- 右侧结果区域
- 扫描结果汇总卡片
- 重复文件列表
- 文件类型与大小筛选
- 删除进度浮层
- 未处理项列表

## 扫描规则

扫描流程和参考项目一致：

1. 选择一个或多个目录
2. 递归扫描目录内文件
3. 跳过符号链接
4. 读取文件基础信息
5. 先按 `文件大小 + 扩展名` 归并候选文件
6. 仅对候选文件计算 `SHA-256`
7. 再按 `文件大小 + Hash` 归并为真正重复文件
8. 生成统计信息、重复分组和跳过项列表

统计结果包括：

- 扫描文件数
- 待比对文件数
- 重复分组数
- 重复文件总数
- 可释放空间

## 交互能力

当前版本已经补齐并扩展了以下交互能力：

- 目录列表支持去重、移除和清空
- 支持显示扫描阶段、百分比和当前处理目录 / 文件
- 支持在扫描过程中中止任务
- 支持整组勾选、全选、反选、自动选择重复项
- 支持重复文件分组折叠
- 支持按文件类型筛选：视频、音频、图片
- 支持按文件大小筛选：`0-1M` 到 `10M以上`
- 支持图片缩略图展示；缩略图统一按 `100x100` 小图生成并走缓存
- 支持手动清除缩略图缓存
- 支持预览文件
- 支持打开所在文件夹
- 支持把选中文件移到回收站 / 废纸篓
- 支持在删除过程中中止任务
- 删除进度弹层会同时显示百分比和 `1/10` 这类数量进度
- 图片重复项如果来自不同目录，会强调显示完整目录路径，方便区分来源

## 平台行为差异

- macOS：
  - 目录选择使用 `osascript`
  - 文件预览使用 Quick Look（`qlmanage -p`）
  - 打开所在位置使用 Finder 定位（`open -R`）
  - 删除使用 Finder 废纸篓
- Windows：
  - 目录选择使用原生多选文件夹选择器
  - 文件预览和打开所在位置使用原生 Shell 打开
  - 删除使用回收站接口
  - 为了减少单文件重复系统调用，删除任务会按批次提交到回收站

## 与参考项目的对应关系

| 参考项目 `findFile` | 当前项目 `CompareFile` |
| --- | --- |
| `src/index.html` | `frontend/index.html` |
| `src/styles.css` | `frontend/styles.css` |
| `src/renderer.js` | `frontend/renderer.js` |
| `src/main.js` | `main.go` + `app.go` + `platform_*.go` |
| `src/preload.js` | `app.go` 中的 JS Bridge |
| `src/scan-service.js` | `internal/scanner/scanner.go` |
| `electron-builder` 打包 | `scripts/package-mac.sh` / `scripts/package-win.sh` |

## 技术结构

```mermaid
flowchart LR
  UI[HTML/CSS/JS] --> Bridge[webview JS Bridge]
  Bridge --> App[Go Desktop App]
  App --> Scan[Go Scanner]
  App --> Dialog[平台原生目录选择]
  App --> Media[缩略图生成与缓存]
  App --> Shell[Finder / Shell / Quick Look / open]
```

```mermaid
flowchart TD
  A[选择目录] --> B[递归扫描文件]
  B --> C[按 大小+扩展名 分组]
  C --> D[候选文件计算 SHA-256]
  D --> E[按 大小+Hash 聚合]
  E --> F[生成重复分组与统计]
```

## 目录说明

- `main.go`：应用入口和窗口启动
- `app.go`：桌面壳能力、静态资源服务、前后端桥接
- `platform_darwin.go`：macOS 目录选择、预览、定位、废纸篓逻辑
- `platform_windows.go`：Windows 目录选择、预览、定位、回收站逻辑
- `internal/scanner/scanner.go`：重复文件扫描逻辑
- `internal/scanner/creation_time_darwin.go`：macOS 创建时间读取
- `internal/scanner/creation_time_default.go`：非 macOS 平台时间兜底
- `frontend/`：界面、样式和前端交互
- `scripts/package-mac.sh`：构建并打包 `.app`
- `scripts/package-win.sh`：构建并打包 `.exe`
- `.tools/`：项目内使用的本地 Go / Zig 工具链
- `build/`：中间构建产物
- `dist/`：最终产物输出目录

## 环境依赖

- macOS
- Go 1.26
- Xcode Command Line Tools
- WebKit（macOS 自带）
- Windows 11 运行时默认依赖系统内置的 WebView2 Runtime
- Windows 构建需要启用 CGO，并提供可用的 C 编译器（例如 MinGW 或项目内置 Zig 工具链）

## 运行要求

- macOS 端使用系统 WebKit
- Windows 11 端使用系统 WebView2 Runtime
- 当前仓库已经提供打包脚本，优先使用项目内 `.tools` 下的本地工具链
- Windows 端已改为优先使用原生系统 API，避免依赖 PowerShell 处理目录选择、打开位置或回收站删除

## 开发步骤

```bash
export PATH="$PWD/.tools/go/bin:$PATH"
go mod tidy
go run .
```

如果没有预置工具链，也可以直接使用系统 Go。

## 开发运行

```bash
go mod tidy
go run .
```

## 打包 macOS `.app`

```bash
./scripts/package-mac.sh
```

产物默认输出到：

`dist/Duplicate Finder.app`

## 打包 Windows 11 `.exe`

仓库内保留了 `scripts/package-win.sh`，同时当前开发环境也支持直接使用本地工具链构建：

```powershell
$env:CC="D:\AI\CompareFile\.tools\zig-0.13\zig-windows-x86_64-0.13.0\zig.exe cc"
$env:CXX="D:\AI\CompareFile\.tools\zig-0.13\zig-windows-x86_64-0.13.0\zig.exe c++"
$env:CGO_ENABLED="1"
$env:GOOS="windows"
$env:GOARCH="amd64"
& "D:\AI\CompareFile\.tools\go\bin\go.exe" build -ldflags="-H windowsgui" -o "dist/windows-11/DuplicateFinder.exe" .
```

产物默认输出到：

`dist/windows-11/DuplicateFinder.exe`

## 产物说明

- macOS：
  `dist/Duplicate Finder.app`
- Windows 11：
  `dist/windows-11/DuplicateFinder.exe`

## 当前实现说明

当前版本已经完成：

- 参考项目界面结构迁移
- 重复文件扫描逻辑迁移
- 海量结果列表虚拟渲染，降低滚动卡顿
- 重复文件工具栏吸顶，并且仅在滚动后显示阴影
- Windows 原生多选目录选择
- Windows 原生打开所在文件夹与回收站删除
- 扫描与删除的协作式中止
- 删除批量提交到回收站，并保留失败明细
- 删除进度数量展示
- 图片缩略图小图化、缓存与手动清理
- macOS 桌面运行与 `.app` 打包
- Windows 11 `.exe` 交叉编译
- Windows 资源文件（`.syso`）集成，用于补充图标、版本号和产品信息
