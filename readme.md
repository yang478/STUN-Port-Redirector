# STUN Port Redirector

## 项目简介

本项目旨在通过利用具备动态 IPv4 地址的 VPS 设备实现 STUN 穿透重定向。参考 ie-12 大佬的教程 https://www.bilibili.com/opus/953960881273700352 以及其他网友的经验，使用 Cloudflare可以实现本地获取的 STUN 地址的重定向。然而，在实际应用中发现，Cloudflare 有时会出现不通的情况，导致无法成功重定向到 STUN 地址。为了解决这一问题，使用 Go 语言编写了一个脚本，该脚本包含一个 API 和重定向功能。

### API 功能
- **更新端口**：通过 POST 请求 `http://ip或域名/api/save` 将端口信息保存至 `data.json` 文件中。
- **本地端口监听与重定向**：在本地端口（例如 33331）上设置监听，并将其重定向到通过 `A1.com` 获取的端口。

## 使用方法

### 第一步：配置 STUN 穿透
请按照教程 [「LUCKY STUN穿透」使用 Cloudflare 的页面规则固定和隐藏网页端口](https://www.bilibili.com/opus/953960881273700352) 操作，以实现本地地址的 STUN 穿透，并通过 A 记录获取到动态 IPv4 地址。这样可以利用该端口反向代理本地服务。

### 第二步：构建和启动 Docker 容器
1. **构建镜像**：
   - 使用 `git clone` 或下载代码至具备 Docker 环境的机器。
   - 构建命令：`docker build -t myapp:latest .`

2. **修改 Docker Compose 文件**：
   - 修改 `docker-compose.yml` 文件中的相关信息，包括需要映射到本地的端口。
   - 注意：`- "33331:33331"` 中的后一个 `33331` 需要与 `redirect_mapping.json` 中对应的端口号一致。

3. **启动容器**：
   - 使用命令 `docker-compose up -d` 启动容器。

### 第三步：配置反向代理
在具备公网 IPv4 地址的机器上安装 Lucky，并使用反向代理本地映射到的 `33331-33334` 端口。注意，反向代理的是 `"33331:33331"` 前面的端口。在子规则中，请确保勾选“使用目标地址的 Host 请求头”。

### 第四步：配置 Webhook
1. **设置 Webhook**：
   - 将 `http://ip或域名/api/save` 设置为 Webhook 地址，方法选择 POST。
   - POST 方法用于更新远端的数据。

2. **设置请求头**：
   - 请求头格式如下：
     ```
     Authorization: Bearer your_token
     Content-Type: application/json
     ```

3. **设置请求体**：
   - 请求体内容如下：
     ```json
     {
       "port": #{port}
     }
     ```

4. **防火墙设置**：
   - 确保远端服务器开启防火墙，并打开 API 所需的端口。
   - 需要一个端口用于反向代理服务，可以使用 Lucky、Caddy 或 Nginx。
   - 本地 STUN 穿透工具不限于 Lucky，可根据个人选择使用其他工具。

本项目通过 VPS 重定向替换了 Cloudflare 的重定向功能，代码是在 DeepSeek 的帮助下完成的，但是本人对Go 语言并不熟悉，该项目仅为满足个人小需求。然而，对于 NAT4 等无法 STUN 穿透的情况，建议使用内网穿透来中转。

## 致谢
感谢 DeepSeek 的帮助，lucky的作者古大羊，以及 ie-12 大佬和众多网友的教程帮助。

---
**注意**：此项目为个人需求开发，使用时请遵守相关法律法规。