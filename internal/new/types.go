package new

import (
	"sync/atomic"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
)

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
	MetadataLength     int
}

type ServiceInstance struct {
	ServiceName string
	IP          string
	Port        uint64
	Metadata    map[string]string
}

type SimulationClient struct {
	clientId            int
	machineId           int
	namingClient        naming_client.INamingClient
	registeredInstances []ServiceInstance
	subscribedServices  []string
	shouldChurn         bool
	churnIndex          int
}

type SimulationStats struct {
	RegisterSuccess   atomic.Int64
	RegisterFail      atomic.Int64
	SubscribeSuccess  atomic.Int64
	SubscribeFail     atomic.Int64
	PushReceived      atomic.Int64
	ChurnSuccess      atomic.Int64
	ChurnFail         atomic.Int64
	DeregisterSuccess atomic.Int64
	DeregisterFail    atomic.Int64
}
