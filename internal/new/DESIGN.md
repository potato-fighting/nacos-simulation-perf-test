# Nacos 大规模仿真测试工具设计文档

## 一、测试目标

模拟线上环境的服务规模对 Nacos 集群进行仿真测试，验证集群在大规模场景下的性能表现。

**测试规模：**
- 200台施压机
- 每台500个线程（模拟500个客户端）
- 总计：10万个客户端、10万个服务、50万个服务实例

---

## 二、测试场景

### 场景1：大规模服务注册后达到稳定状态

**测试流程：**
1. 所有客户端并发注册服务（每个客户端注册5个服务实例）
2. 所有客户端订阅服务（每个客户端订阅5个服务）
3. 达到稳定状态后持续观察20分钟
4. 关闭所有施压机，观察注销性能

**观察指标：**
- 注册过程中的服务端性能指标
- 推送SLA（延迟、成功率）
- 稳定状态下的服务端性能指标
- 注销过程中的服务端性能指标

### 场景2：稳定状态后部分实例频繁发布

**测试流程：**
1. 执行场景1的步骤1-3
2. 稳定状态后，选择10%-20%的实例进行周期性变更
3. 每隔10秒进行一次变更（注销后重新注册）
4. 持续观察20分钟

**观察指标：**
- 变更过程中的推送性能
- 服务端处理变更的性能指标
- 未变更实例的稳定性

---

## 三、命名和分配策略

### 1. 服务命名规则

**服务池设计：**
- 总服务数：100,000个
- 服务名格式：`nacos.sim.svc.{machineId}.{serviceIndex}`
- 每台机器负责：500个服务（100,000 / 200 = 500）

**示例：**
```
机器1: nacos.sim.svc.1.0 ~ nacos.sim.svc.1.499
机器2: nacos.sim.svc.2.0 ~ nacos.sim.svc.2.499
...
机器200: nacos.sim.svc.200.0 ~ nacos.sim.svc.200.499
```

### 2. 实例命名规则

**实例标识：**
- IP地址：`10.{machineId/256}.{machineId%256}.{threadId%256}`
- 端口：`8080 + (threadId % 1000)`
- 元数据：包含 machineId、threadId、timestamp

**示例：**
```
机器1，线程0: IP=10.0.1.0, Port=8080
机器1，线程1: IP=10.0.1.1, Port=8081
机器200，线程499: IP=10.0.200.243, Port=8579
```

### 3. 线程服务分配策略

**每个线程注册5个服务实例：**
```
计算公式：
  serviceIndex = threadId * 5 + instanceOffset
  其中 instanceOffset ∈ [0, 4]

示例：
  线程0 -> 注册服务: svc.{machineId}.0, svc.{machineId}.1, svc.{machineId}.2, svc.{machineId}.3, svc.{machineId}.4
  线程1 -> 注册服务: svc.{machineId}.5, svc.{machineId}.6, svc.{machineId}.7, svc.{machineId}.8, svc.{machineId}.9
  线程499 -> 注册服务: svc.{machineId}.2495, svc.{machineId}.2496, svc.{machineId}.2497, svc.{machineId}.2498, svc.{machineId}.2499
```

**每个线程订阅5个随机服务：**
```
- 从全局服务池（10万个服务）中随机选择5个
- 使用 (machineId * 1000 + threadId) 作为随机种子，保证可重现
- 订阅的服务可能来自任意机器
```

---

## 四、稳态判断和变更策略

### 1. 稳态判断机制

**注册完成标志：**
- 所有线程完成初始注册（使用 sync.WaitGroup 同步）
- 所有线程完成服务订阅
- 输出日志：`[Phase] Registration completed, entering stable phase`

**稳态等待期：**
- 注册完成后，等待固定时间让推送稳定（建议5-10分钟）
- 期间所有实例保持注册状态，持续监听推送
- 定期输出统计信息（每30秒）

**稳态持续时间：**
- 由参数 `--stableDuration` 控制（默认1200秒，即20分钟）

### 2. 变更实例选择策略（场景2）

**选择规则：**
```
变更比例：churnRatio（默认0.15，即15%）
变更线程数：500 * churnRatio = 75个线程

判断公式：
  shouldChurn = (threadId % 100) < (churnRatio * 100)

示例（churnRatio=0.15）：
  线程0: 0 % 100 = 0 < 15 ✓ 参与变更
  线程1: 1 % 100 = 1 < 15 ✓ 参与变更
  ...
  线程14: 14 % 100 = 14 < 15 ✓ 参与变更
  线程15: 15 % 100 = 15 < 15 ✗ 不参与
  ...
  线程100: 0 % 100 = 0 < 15 ✓ 参与变更
```

**变更操作：**
```
每隔 churnInterval 秒（默认10秒）：
  1. 选择一个已注册的实例（5个实例轮流）
  2. 注销该实例
  3. 等待1秒
  4. 重新注册该实例（保持相同的服务名、IP、端口）
```

### 3. 执行阶段流程

**阶段1：Registering（注册阶段）**
```
1. 创建500个客户端（goroutine）
2. 每个客户端并发执行：
   - 注册5个服务实例
   - 订阅5个随机服务
3. 使用 WaitGroup 等待所有客户端完成
4. 输出统计：注册成功/失败数、耗时
```

**阶段2：Stable（稳定阶段）**
```
1. 所有实例保持注册状态
2. 持续监听订阅的服务变更
3. 定期输出统计信息（每30秒）
4. 持续时间：stableDuration
```

**阶段3：Churning（变更阶段，仅场景2）**
```
1. 识别需要变更的线程（基于 churnRatio）
2. 变更线程每隔 churnInterval 秒执行变更操作
3. 非变更线程保持稳定
4. 持续时间：stableDuration
```

**阶段4：Shutdown（关闭阶段）**
```
1. 所有客户端注销所有实例
2. 观察服务端注销性能
3. 输出最终统计信息
4. 程序退出
```

---

## 五、测试命令

### 场景1：大规模服务注册后达到稳定状态

```bash
# 在200台施压机上分别执行（machineId从1到200）
./nacos-bench \
  --nacosServerAddr=192.168.1.100,192.168.1.101,192.168.1.102 \
  --perfMode=simulation \
  --perfApi=largeScaleStable \
  --nacosClientCount=500 \
  --serviceCount=100000 \
  --registerPerClient=5 \
  --subscribePerClient=5 \
  --stableDuration=1200 \
  --machineId=1
```

**参数说明：**
- `nacosServerAddr`: Nacos集群地址（逗号分隔）
- `perfMode`: 测试模式，固定为 `simulation`
- `perfApi`: 测试场景，`largeScaleStable` 表示场景1
- `nacosClientCount`: 每台机器的客户端数（500）
- `serviceCount`: 全局服务总数（100000）
- `registerPerClient`: 每个客户端注册的服务数（5）
- `subscribePerClient`: 每个客户端订阅的服务数（5）
- `stableDuration`: 稳定状态持续时间（秒）
- `machineId`: 当前机器编号（1-200）

### 场景2：稳定状态后部分实例频繁发布

```bash
# 在200台施压机上分别执行（machineId从1到200）
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
  --machineId=1
```

**新增参数：**
- `perfApi`: `largeScaleWithChurn` 表示场景2
- `churnRatio`: 变更实例比例（0.1-0.2，默认0.15）
- `churnInterval`: 变更间隔秒数（默认10）

---

## 六、核心数据结构

```go
// 仿真配置
type SimulationConfig struct {
    NacosAddr          string
    MachineId          int
    ClientCount        int
    ServiceCount       int
    RegisterPerClient  int
    SubscribePerClient int
    StableDuration     int
    ChurnRatio         float64
    ChurnInterval      int
    PerfApi            string
}

// 客户端
type SimulationClient struct {
    clientId            int
    machineId           int
    namingClient        naming_client.INamingClient
    registeredInstances []ServiceInstance
    subscribedServices  []string
    shouldChurn         bool
    churnIndex          int
}

// 服务实例
type ServiceInstance struct {
    ServiceName string
    IP          string
    Port        uint64
    Metadata    map[string]string
}

// 统计信息
type SimulationStats struct {
    RegisterSuccess    atomic.Int64
    RegisterFail       atomic.Int64
    SubscribeSuccess   atomic.Int64
    SubscribeFail      atomic.Int64
    PushReceived       atomic.Int64
    ChurnSuccess       atomic.Int64
    ChurnFail          atomic.Int64
    DeregisterSuccess  atomic.Int64
    DeregisterFail     atomic.Int64
}
```

---

## 七、统计输出

**注册阶段输出：**
```
[Registering] Progress: 500/500 clients completed
[Registering] Register Success: 2500, Fail: 0, Duration: 45s
[Registering] Subscribe Success: 2500, Fail: 0
```

**稳定阶段输出（每30秒）：**
```
[Stable] Duration: 30s, Push Received: 1234
[Stable] Duration: 60s, Push Received: 2456
...
```

**变更阶段输出（每30秒）：**
```
[Churning] Duration: 30s, Churn Success: 225, Fail: 0, Push Received: 3456
[Churning] Duration: 60s, Churn Success: 450, Fail: 0, Push Received: 6789
...
```

**关闭阶段输出：**
```
[Shutdown] Deregister Success: 2500, Fail: 0, Duration: 10s
[Summary] Total Register: 2500, Subscribe: 2500, Churn: 1800, Deregister: 2500
```

---

## 八、实施步骤

1. **准备环境**
   - 准备200台施压机
   - 部署 Nacos 集群
   - 编译测试工具

2. **执行场景1**
   - 在所有施压机上同时启动测试工具（machineId 1-200）
   - 观察注册过程性能
   - 等待稳定状态20分钟
   - 同时关闭所有施压机
   - 收集服务端指标

3. **执行场景2**
   - 重启 Nacos 集群（清空数据）
   - 在所有施压机上启动测试工具（使用 largeScaleWithChurn）
   - 观察注册和稳定阶段
   - 观察变更阶段的推送性能
   - 收集服务端指标

4. **数据分析**
   - 汇总所有施压机的统计数据
   - 分析服务端性能指标
   - 生成测试报告
