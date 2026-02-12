# Termia /bin Wrapper نصب与使用

## 目标
将编译好的 `termia` 安装为 `/bin/termia`（Linux/WSL）或 `/usr/local/bin/termia`（macOS），让它作为透明的 bash/zsh 前门。

## 安装

### Linux / WSL2
```bash
sudo scripts/install-termia.sh --bin ./termia --target /bin
```

### macOS
```bash
sudo scripts/install-termia.sh --bin ./termia --target /usr/local/bin
```

> 提示：安装脚本不会修改任何 shell RC 文件。

## 使用

### 交互式（会记录）
```bash
/bin/termia
```

### 非交互式（不记录）
```bash
/bin/termia -c "echo hello"
/bin/termia -c "exit 42"; echo $?
```

## 数据目录
默认数据目录为：
```
${XDG_DATA_HOME:-$HOME/.local/share}/termia
```
其中包含：
- `db/history.db`
- `transcripts/`
- `shell/`

## 卸载
```bash
sudo rm -f /bin/termia
sudo rm -f /usr/local/bin/termia
```

## 相关脚本
Shell 集成脚本位于：
- `scripts/termia.bash`
- `scripts/termia.zsh`
