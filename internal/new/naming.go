package new

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

var (
	simClients []*SimulationClient
	stats      SimulationStats
	phase      string
)

func generateServiceName(machineId, serviceIndex int) string {
	return fmt.Sprintf("nacos.sim.svc.%d.%d", machineId, serviceIndex)
}

func generateInstanceIP(machineId, threadId int) string {
	return fmt.Sprintf("10.%d.%d.%d", machineId/256, machineId%256, threadId%256)
}

func generateMetadata(machineId, threadId int, length int) map[string]string {
	value := strings.Repeat("x", length)
	return map[string]string{
		"machineId": fmt.Sprintf("%d", machineId),
		"threadId":  fmt.Sprintf("%d", threadId),
		"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
		"data":      value,
	}
}

func InitSimulation(config SimulationConfig) {
	fmt.Printf("[Init] Starting simulation on machine %d\n", config.MachineId)

	clientConfig := constant.ClientConfig{
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/log",
		CacheDir:            "/tmp/nacos/cache",
		LogLevel:            "warn",
	}

	serverConfigs := []constant.ServerConfig{}
	for _, addr := range strings.Split(config.NacosAddr, ",") {
		serverConfigs = append(serverConfigs, constant.ServerConfig{
			IpAddr:      addr,
			ContextPath: "/nacos",
			Port:        8848,
		})
	}

	var wg sync.WaitGroup
	clientsMutex := sync.Mutex{}

	for i := 0; i < config.ClientCount; i++ {
		wg.Add(1)
		go func(threadId int) {
			defer wg.Done()

			nacosClient, err := clients.NewNamingClient(vo.NacosClientParam{
				ClientConfig:  &clientConfig,
				ServerConfigs: serverConfigs,
			})
			if err != nil {
				stats.RegisterFail.Add(1)
				return
			}

			simClient := &SimulationClient{
				clientId:            threadId,
				machineId:           config.MachineId,
				namingClient:        nacosClient,
				registeredInstances: []ServiceInstance{},
				subscribedServices:  []string{},
				shouldChurn:         (threadId % 100) < int(config.ChurnRatio*100),
				churnIndex:          0,
			}

			clientsMutex.Lock()
			simClients = append(simClients, simClient)
			clientsMutex.Unlock()
		}(i)
	}

	wg.Wait()
	fmt.Printf("[Init] Created %d clients\n", len(simClients))
}

func RunRegistration(config SimulationConfig) {
	phase = "Registering"
	fmt.Println("[Registering] Starting registration phase...")
	startTime := time.Now()

	var wg sync.WaitGroup
	for _, client := range simClients {
		wg.Add(1)
		go func(c *SimulationClient) {
			defer wg.Done()
			registerInstances(c, config)
			subscribeServices(c, config)
		}(client)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("[Registering] Completed in %v\n", duration)
	fmt.Printf("[Registering] Register Success: %d, Fail: %d\n", stats.RegisterSuccess.Load(), stats.RegisterFail.Load())
	fmt.Printf("[Registering] Subscribe Success: %d, Fail: %d\n", stats.SubscribeSuccess.Load(), stats.SubscribeFail.Load())
}

func registerInstances(client *SimulationClient, config SimulationConfig) {
	ip := generateInstanceIP(client.machineId, client.clientId)
	port := uint64(8080 + (client.clientId % 1000))

	for i := 0; i < config.RegisterPerClient; i++ {
		serviceIndex := client.clientId*config.RegisterPerClient + i
		serviceName := generateServiceName(client.machineId, serviceIndex)

		instance := ServiceInstance{
			ServiceName: serviceName,
			IP:          ip,
			Port:        port,
			Metadata:    generateMetadata(client.machineId, client.clientId, config.MetadataLength),
		}

		_, err := client.namingClient.RegisterInstance(vo.RegisterInstanceParam{
			Ip:          instance.IP,
			Port:        instance.Port,
			ServiceName: instance.ServiceName,
			Weight:      1.0,
			Enable:      true,
			Healthy:     true,
			Metadata:    instance.Metadata,
			Ephemeral:   true,
		})

		if err != nil {
			stats.RegisterFail.Add(1)
		} else {
			stats.RegisterSuccess.Add(1)
			client.registeredInstances = append(client.registeredInstances, instance)
		}
	}
}

func subscribeServices(client *SimulationClient, config SimulationConfig) {
	rnd := rand.New(rand.NewSource(int64(client.machineId*1000 + client.clientId)))

	for i := 0; i < config.SubscribePerClient; i++ {
		randomMachineId := rnd.Intn(200) + 1
		randomServiceIndex := rnd.Intn(500)
		serviceName := generateServiceName(randomMachineId, randomServiceIndex)

		err := client.namingClient.Subscribe(&vo.SubscribeParam{
			ServiceName: serviceName,
			GroupName:   "DEFAULT_GROUP",
			SubscribeCallback: func(services []model.Instance, err error) {
				if err == nil {
					stats.PushReceived.Add(1)
				}
			},
		})

		if err != nil {
			stats.SubscribeFail.Add(1)
		} else {
			stats.SubscribeSuccess.Add(1)
			client.subscribedServices = append(client.subscribedServices, serviceName)
		}
	}
}

func RunStablePhase(config SimulationConfig) {
	phase = "Stable"
	fmt.Printf("[Stable] Entering stable phase for %d seconds...\n", config.StableDuration)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	endTime := startTime.Add(time.Duration(config.StableDuration) * time.Second)

	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			elapsed := int(time.Since(startTime).Seconds())
			fmt.Printf("[Stable] Duration: %ds, Push Received: %d\n", elapsed, stats.PushReceived.Load())
		case <-time.After(time.Until(endTime)):
			return
		}
	}

	fmt.Printf("[Stable] Phase completed. Total Push Received: %d\n", stats.PushReceived.Load())
}

func RunChurnPhase(config SimulationConfig) {
	phase = "Churning"
	fmt.Printf("[Churning] Starting churn phase for %d seconds...\n", config.StableDuration)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	endTime := startTime.Add(time.Duration(config.StableDuration) * time.Second)

	for _, client := range simClients {
		if client.shouldChurn {
			go churnLoop(client, config, endTime)
		}
	}

	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			elapsed := int(time.Since(startTime).Seconds())
			fmt.Printf("[Churning] Duration: %ds, Churn Success: %d, Fail: %d, Push Received: %d\n",
				elapsed, stats.ChurnSuccess.Load(), stats.ChurnFail.Load(), stats.PushReceived.Load())
		case <-time.After(time.Until(endTime)):
			return
		}
	}

	fmt.Printf("[Churning] Phase completed\n")
}

func churnLoop(client *SimulationClient, config SimulationConfig, endTime time.Time) {
	ticker := time.NewTicker(time.Duration(config.ChurnInterval) * time.Second)
	defer ticker.Stop()

	for time.Now().Before(endTime) {
		<-ticker.C
		if time.Now().After(endTime) {
			return
		}

		if len(client.registeredInstances) == 0 {
			continue
		}

		instance := client.registeredInstances[client.churnIndex]
		client.churnIndex = (client.churnIndex + 1) % len(client.registeredInstances)

		_, err := client.namingClient.DeregisterInstance(vo.DeregisterInstanceParam{
			Ip:          instance.IP,
			Port:        instance.Port,
			ServiceName: instance.ServiceName,
			Ephemeral:   true,
		})

		if err != nil {
			stats.ChurnFail.Add(1)
			continue
		}

		time.Sleep(1 * time.Second)

		_, err = client.namingClient.RegisterInstance(vo.RegisterInstanceParam{
			Ip:          instance.IP,
			Port:        instance.Port,
			ServiceName: instance.ServiceName,
			Weight:      1.0,
			Enable:      true,
			Healthy:     true,
			Metadata:    instance.Metadata,
			Ephemeral:   true,
		})

		if err != nil {
			stats.ChurnFail.Add(1)
		} else {
			stats.ChurnSuccess.Add(1)
		}
	}
}

func RunShutdown(config SimulationConfig) {
	phase = "Shutdown"
	fmt.Println("[Shutdown] Starting shutdown phase...")
	startTime := time.Now()

	var wg sync.WaitGroup
	for _, client := range simClients {
		wg.Add(1)
		go func(c *SimulationClient) {
			defer wg.Done()
			for _, instance := range c.registeredInstances {
				_, err := c.namingClient.DeregisterInstance(vo.DeregisterInstanceParam{
					Ip:          instance.IP,
					Port:        instance.Port,
					ServiceName: instance.ServiceName,
					Ephemeral:   true,
				})
				if err != nil {
					stats.DeregisterFail.Add(1)
				} else {
					stats.DeregisterSuccess.Add(1)
				}
			}
		}(client)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("[Shutdown] Completed in %v\n", duration)
	fmt.Printf("[Shutdown] Deregister Success: %d, Fail: %d\n", stats.DeregisterSuccess.Load(), stats.DeregisterFail.Load())
	printSummary()
}

func printSummary() {
	fmt.Println("\n[Summary] Final Statistics:")
	fmt.Printf("  Register Success: %d, Fail: %d\n", stats.RegisterSuccess.Load(), stats.RegisterFail.Load())
	fmt.Printf("  Subscribe Success: %d, Fail: %d\n", stats.SubscribeSuccess.Load(), stats.SubscribeFail.Load())
	fmt.Printf("  Push Received: %d\n", stats.PushReceived.Load())
	fmt.Printf("  Churn Success: %d, Fail: %d\n", stats.ChurnSuccess.Load(), stats.ChurnFail.Load())
	fmt.Printf("  Deregister Success: %d, Fail: %d\n", stats.DeregisterSuccess.Load(), stats.DeregisterFail.Load())
}
