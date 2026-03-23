# Nacos 大规模仿真测试工具使用指南

## 快速开始

### 1. 编译

```bash
cd nacos-bench
go build -o nacos-bench cmd/main/main_new.go
```

### 2. 场景1：大规模服务注册后达到稳定状态

在200台施压机上分别执行（修改 machineId 从 1 到 200）：

```bash
./nacos-bench \
  --nacosServerAddr=192.168.1.100,192.168.1.101,192.168.1.102 \
  --perfMode=simulation \
  --perfApi=largeScaleStable \
  --nacosClientCount=500 \
  --serviceCount=100000 \
  --registerPerClient=5 \
  --subscribePerClient=5 \
  --stableDuration=1200 \
  --namingMetadataLength=128 \
  --machineId=1
```

### 3. 场景2：稳定状态后部分实例频繁发布

```bash
./nacos-bench \
  --nacosServerAddr=192.168.1.100,192.168.1.101,192.168.1.102 \
  --perfMode=simulation \
  --perfApi=largeScaleWithChurn \
  --nacosClientCount=500 \
  --serviceCount=100000 \
  --registerPerClient=5 \
  --subscribePerClient=5 \
  --stableDuration=1200 \
  --churnRatio=0.15 \
  --churnInterval=10 \
  --namingMetadataLength=128 \
  --machineId=1
```

## 参数说明

| 参数 | 说明 | 默认值 | 场景1 | 场景2 |
|------|------|--------|-------|-------|
| nacosServerAddr | Nacos集群地址（逗号分隔） | 127.0.0.1 | ✓ | ✓ |
| perfMode | 测试模式，固定为 simulation | naming | ✓ | ✓ |
| perfApi | 测试场景 | namingReg | largeScaleStable | largeScaleWithChurn |
| machineId | 机器编号（1-200） | 1 | ✓ | ✓ |
| nacosClientCount | 每台机器的客户端数 | 1000 | 500 | 500 |
| serviceCount | 全局服务总数 | 15000 | 100000 | 100000 |
| registerPerClient | 每个客户端注册的服务数 | 3 | 5 | 5 |
| subscribePerClient | 每个客户端订阅的服务数 | 3 | 5 | 5 |
| stableDuration | 稳定状态持续时间（秒） | 1200 | 1200 | 1200 |
| churnRatio | 变更实例比例（0.1-0.2） | 0.15 | - | 0.15 |
| churnInterval | 变更间隔（秒） | 10 | - | 10 |
| namingMetadataLength | 元数据长度 | 128 | 128 | 128 |

## 测试流程

### 场景1执行流程

1. **初始化阶段**：创建500个客户端
2. **注册阶段**：每个客户端注册5个服务实例，订阅5个服务
3. **稳定阶段**：保持20分钟，观察推送和性能
4. **关闭阶段**：按 Ctrl+C 触发，注销所有实例

### 场景2执行流程

1. **初始化阶段**：创建500个客户端
2. **注册阶段**：每个客户端注册5个服务实例，订阅5个服务
3. **稳定阶段**：保持20分钟，观察推送和性能
4. **变更阶段**：15%的客户端每10秒变更一次实例，持续20分钟
5. **关闭阶段**：按 Ctrl+C 触发，注销所有实例

## 输出示例

```
[Init] Starting simulation on machine 1
[Init] Created 500 clients
[Registering] Starting registration phase...
[Registering] Completed in 45s
[Registering] Register Success: 2500, Fail: 0
[Registering] Subscribe Success: 2500, Fail: 0
[Stable] Entering stable phase for 1200 seconds...
[Stable] Duration: 30s, Push Received: 1234
[Stable] Duration: 60s, Push Received: 2456
...
[Stable] Phase completed. Total Push Received: 45678
[Churning] Starting churn phase for 1200 seconds...
[Churning] Duration: 30s, Churn Success: 225, Fail: 0, Push Received: 46789
...
[Signal] Received shutdown signal
[Shutdown] Starting shutdown phase...
[Shutdown] Completed in 10s
[Shutdown] Deregister Success: 2500, Fail: 0

[Summary] Final Statistics:
  Register Success: 2500, Fail: 0
  Subscribe Success: 2500, Fail: 0
  Push Received: 89012
  Churn Success: 1800, Fail: 0
  Deregister Success: 2500, Fail: 0
```

## 批量部署脚本

创建 `deploy.sh` 用于批量启动：

```bash
#!/bin/bash

NACOS_ADDR="192.168.1.100,192.168.1.101,192.168.1.102"
MACHINE_ID=$1
SCENARIO=${2:-largeScaleStable}

if [ -z "$MACHINE_ID" ]; then
    echo "Usage: $0 <machine_id> [scenario]"
    echo "  scenario: largeScaleStable | largeScaleWithChurn"
    exit 1
fi

./nacos-bench \
  --nacosServerAddr=$NACOS_ADDR \
  --perfMode=simulation \
  --perfApi=$SCENARIO \
  --nacosClientCount=500 \
  --serviceCount=100000 \
  --registerPerClient=5 \
  --subscribePerClient=5 \
  --stableDuration=1200 \
  --churnRatio=0.15 \
  --churnInterval=10 \
  --namingMetadataLength=128 \
  --machineId=$MACHINE_ID
```

使用方式：
```bash
# 场景1
./deploy.sh 1 largeScaleStable

# 场景2
./deploy.sh 1 largeScaleWithChurn
```

## 注意事项

1. **machineId 必须唯一**：每台机器使用不同的 machineId（1-200）
2. **同时启动**：所有施压机应尽量同时启动以模拟真实场景
3. **资源监控**：测试期间监控 Nacos 服务端的 CPU、内存、网络等指标
4. **日志收集**：建议将输出重定向到文件：`./nacos-bench ... > machine1.log 2>&1`
5. **优雅关闭**：使用 Ctrl+C 触发优雅关闭，观察注销性能
