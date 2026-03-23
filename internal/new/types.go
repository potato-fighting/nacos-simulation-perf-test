package new

import (
	"sync/atomic"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
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
	ChurnDuration      int
	ChurnRatio         float64
	ChurnInterval      int
	PerfApi            string
	MetadataLength     int
	Debug              bool
	ConfigCount        int
	ConfigContentLength int
	ConfigListenPerClient int
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
	configClient        config_client.IConfigClient
	instance            ServiceInstance
	subscribedServices  []string
	listenedConfigs     []string
	shouldChurn         bool
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
	ConfigPublishSuccess atomic.Int64
	ConfigPublishFail    atomic.Int64
	ConfigListenSuccess  atomic.Int64
	ConfigListenFail     atomic.Int64
	ConfigPushReceived   atomic.Int64
}
