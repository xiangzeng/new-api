# Docker Compose 部署指南

## 说明

使用 Docker Compose 部署 New API，包含 PostgreSQL 数据库和 Redis 缓存。适用于个人或小团队的独立部署场景。

## 前置要求

| 项目     | 要求                                 |
| -------- | ------------------------------------ |
| 操作系统 | Linux（CentOS 7+、Ubuntu 18.04+、Debian 10+） |
| Docker   | ≥ 20.10                             |
| Docker Compose | ≥ 2.0（`docker compose` 子命令） |
| 服务器配置 | 至少 1 核 2G 内存                  |

## 部署步骤

### 1. 创建目录并写入配置

```bash
mkdir -p ~/new-api

cat > ~/new-api/docker-compose.yml << 'EOF'
services:
  new-api:
    image: ghcr.io/xiangzeng/new-api:beta
    container_name: new-api
    restart: always
    command: --log-dir /app/logs
    ports:
      - "3000:3000"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    environment:
      - SQL_DSN=postgresql://root:你的数据库密码@postgres:5432/new-api
      - REDIS_CONN_STRING=redis://redis
      - SESSION_SECRET=你的会话密钥
      - TZ=Asia/Shanghai
      - BATCH_UPDATE_ENABLED=true
    depends_on:
      - redis
      - postgres
    networks:
      - new-api-network

  redis:
    image: redis:latest
    container_name: redis
    restart: always
    networks:
      - new-api-network

  postgres:
    image: postgres:15
    container_name: postgres
    restart: always
    environment:
      POSTGRES_USER: root
      POSTGRES_PASSWORD: 你的数据库密码
      POSTGRES_DB: new-api
    volumes:
      - pg_data:/var/lib/postgresql/data
    networks:
      - new-api-network

volumes:
  pg_data:

networks:
  new-api-network:
    driver: bridge
EOF
```

> **注意：** 两处 `你的数据库密码` 必须保持一致，这是内部数据库密码，不对外暴露。

### 2. 生成密钥

部署前需要生成 `SESSION_SECRET`，用于会话加密：

```bash
openssl rand -hex 16
```

将生成的值替换 `docker-compose.yml` 中的 `你的会话密钥`。

### 3. 拉取并启动

```bash
cd ~/new-api
docker compose pull
docker compose up -d
```

### 4. 验证运行

```bash
curl http://localhost:3000/api/status
```

返回 `"success":true` 即为成功。

## 访问面板

启动成功后，访问 `http://你的服务器IP:3000` 进入管理面板，首次访问注册的账号即为管理员。

如果服务器未开放公网端口，可通过 SSH 隧道访问：

```bash
ssh -L 3000:127.0.0.1:3000 user@你的服务器IP
```

隧道建立后打开 `http://localhost:3000` 即可。

## 环境变量说明

| 变量名                 | 说明                          | 是否必填 |
| ---------------------- | ----------------------------- | -------- |
| `SQL_DSN`              | 数据库连接字符串              | 可选（默认 SQLite） |
| `REDIS_CONN_STRING`    | Redis 连接字符串              | 可选     |
| `SESSION_SECRET`       | 会话密钥，多机部署必须一致    | **必填** |
| `CRYPTO_SECRET`        | 加密密钥，使用 Redis 时必填   | 条件必填 |
| `TZ`                   | 时区                          | 可选     |
| `BATCH_UPDATE_ENABLED` | 启用批量更新（提升性能）      | 可选     |

## 常用运维命令

```bash
# 查看日志
docker compose logs -f new-api

# 重启服务
docker compose restart

# 更新镜像
docker compose pull && docker compose up -d

# 停止服务
docker compose down
```

## 相关链接

- [GitHub 仓库](https://github.com/xiangzeng/new-api)
