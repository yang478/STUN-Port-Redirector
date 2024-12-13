# 使用官方 Go 镜像作为基础镜像
FROM golang:1.20-alpine AS builder

# 设置阿里云 Go 模块代理
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/,direct

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制项目文件到容器中
COPY . .

# 编译 Go 程序
RUN go build -o redirect_sync .

# 使用轻量级 alpine 镜像作为运行环境
FROM alpine:latest

# 设置工作目录
WORKDIR /app

# 复制编译后的二进制文件
COPY --from=builder /app/redirect_sync .

# 复制 JSON 文件到镜像中
COPY data.json ./
COPY redirect_mapping.json ./

# 暴露端口（根据你的程序需要）
EXPOSE 5000 43881 43882 43883 43884

# 设置环境变量（如果需要）
ENV BEARER_TOKEN="lHh_hEdJ9boxENqG1eLeKAUhxgopJgZMhjoHGuD0e1Q"

# 运行 Go 程序
CMD ["./redirect_sync"]
