package new

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
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

	for threadId := 0; threadId < config.ClientCount; threadId++ {
		serviceName := generateServiceName(config.MachineId, threadId)

		for instanceIdx := 0; instanceIdx < config.RegisterPerClient; instanceIdx++ {
			wg.Add(1)
			go func(tid, idx int) {
				defer wg.Done()

				nacosClient, err := clients.NewNamingClient(vo.NacosClientParam{
					ClientConfig:  &clientConfig,
					ServerConfigs: serverConfigs,
				})
				if err != nil {
					stats.RegisterFail.Add(1)
					fmt.Printf("[Error] Failed to create naming client: %v\n", err)
					return
				}

				var configClient config_client.IConfigClient
				if config.ConfigCount > 0 {
					configClient, err = clients.NewConfigClient(vo.NacosClientParam{
						ClientConfig:  &clientConfig,
						ServerConfigs: serverConfigs,
					})
					if err != nil {
						fmt.Printf("[Error] Failed to create config client: %v\n", err)
					}
				}

				ip := fmt.Sprintf("10.%d.%d.%d", config.MachineId/256, config.MachineId%256, (tid*config.RegisterPerClient+idx)%256)
				port := uint64(8080 + idx)

				simClient := &SimulationClient{
					clientId:           tid,
					machineId:          config.MachineId,
					namingClient:       nacosClient,
					configClient:       configClient,
					instance:           ServiceInstance{
						ServiceName: serviceName,
						IP:          ip,
						Port:        port,
						Metadata:    generateMetadata(config.MachineId, tid, config.MetadataLength),
					},
					subscribedServices: []string{},
					listenedConfigs:    []string{},
					shouldChurn:        (tid % 100) < int(config.ChurnRatio*100),
				}

				clientsMutex.Lock()
				simClients = append(simClients, simClient)
				clientsMutex.Unlock()
			}(threadId, instanceIdx)
		}
	}

	wg.Wait()
	fmt.Printf("[Init] Created %d clients\n", len(simClients))

	churnCount := 0
	for _, client := range simClients {
		if client.shouldChurn {
			churnCount++
		}
	}
	fmt.Printf("[Init] Clients that will churn: %d (%.1f%%)\n", churnCount, float64(churnCount)*100/float64(len(simClients)))

	fmt.Println("[Init] Waiting for clients to connect...")
	time.Sleep(5 * time.Second)

	if config.ConfigCount > 0 {
		initConfigs(config)
	}
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
			listenConfigs(c, config)
		}(client)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("[Registering] Completed in %v\n", duration)
	fmt.Printf("[Registering] Register Success: %d, Fail: %d\n", stats.RegisterSuccess.Load(), stats.RegisterFail.Load())
	fmt.Printf("[Registering] Subscribe Success: %d, Fail: %d\n", stats.SubscribeSuccess.Load(), stats.SubscribeFail.Load())
	if config.ConfigCount > 0 {
		fmt.Printf("[Registering] Config Listen Success: %d, Fail: %d\n", stats.ConfigListenSuccess.Load(), stats.ConfigListenFail.Load())
	}
}

func registerInstances(client *SimulationClient, config SimulationConfig) {
	_, err := client.namingClient.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          client.instance.IP,
		Port:        client.instance.Port,
		ServiceName: client.instance.ServiceName,
		GroupName:   "DEFAULT_GROUP",
		ClusterName: "DEFAULT",
		Weight:      1.0,
		Enable:      true,
		Healthy:     true,
		Metadata:    client.instance.Metadata,
		Ephemeral:   true,
	})

	if err != nil {
		stats.RegisterFail.Add(1)
		fmt.Printf("[Error] Register failed for service %s: %v\n", client.instance.ServiceName, err)
	} else {
		stats.RegisterSuccess.Add(1)
	}
}

func subscribeServices(client *SimulationClient, config SimulationConfig) {
	if client.instance.Port != 8080 {
		return
	}

	rnd := rand.New(rand.NewSource(int64(client.clientId)))
	subscribedSet := make(map[int]bool)
	subscribedSet[client.clientId] = true

	for i := 0; i < config.SubscribePerClient; i++ {
		var randomThreadId int
		for {
			randomThreadId = rnd.Intn(config.ClientCount)
			if !subscribedSet[randomThreadId] {
				subscribedSet[randomThreadId] = true
				break
			}
		}

		serviceName := generateServiceName(client.machineId, randomThreadId)
		if config.Debug {
			fmt.Printf("[Subscribe] Client %d (IP: %s, Port: %d) subscribing to service: %s\n",
				client.clientId, client.instance.IP, client.instance.Port, serviceName)
		}

		err := client.namingClient.Subscribe(&vo.SubscribeParam{
			ServiceName: serviceName,
			GroupName:   "DEFAULT_GROUP",
			SubscribeCallback: func(services []model.Instance, err error) {
				if err == nil {
					stats.PushReceived.Add(1)
					if config.Debug {
						fmt.Printf("[Push] Service: %s, Instance count: %d, Instances: %v\n", serviceName, len(services), services)
					}
				} else {
					fmt.Printf("[Push Error] Service: %s, Error: %v\n", serviceName, err)
				}
			},
		})

		if err != nil {
			stats.SubscribeFail.Add(1)
			fmt.Printf("[Error] Subscribe failed for service %s: %v\n", serviceName, err)
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
			if config.ConfigCount > 0 {
				fmt.Printf("[Stable] Duration: %ds, Push Received: %d, Config Push Received: %d\n", elapsed, stats.PushReceived.Load(), stats.ConfigPushReceived.Load())
			} else {
				fmt.Printf("[Stable] Duration: %ds, Push Received: %d\n", elapsed, stats.PushReceived.Load())
			}
		case <-time.After(time.Until(endTime)):
			return
		}
	}

	fmt.Printf("[Stable] Phase completed. Total Push Received: %d\n", stats.PushReceived.Load())
}

func RunChurnPhase(config SimulationConfig) {
	phase = "Churning"
	fmt.Printf("[Churning] Starting churn phase for %d seconds...\n", config.ChurnDuration)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	endTime := startTime.Add(time.Duration(config.ChurnDuration) * time.Second)

	for _, client := range simClients {
		if client.shouldChurn {
			go churnLoop(client, config, endTime)
		}
	}

	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			elapsed := int(time.Since(startTime).Seconds())
			if config.ConfigCount > 0 {
				fmt.Printf("[Churning] Duration: %ds, Churn Success: %d, Fail: %d, Push Received: %d, Config Publish: %d, Config Push: %d\n",
					elapsed, stats.ChurnSuccess.Load(), stats.ChurnFail.Load(), stats.PushReceived.Load(),
					stats.ConfigPublishSuccess.Load(), stats.ConfigPushReceived.Load())
			} else {
				fmt.Printf("[Churning] Duration: %ds, Churn Success: %d, Fail: %d, Push Received: %d\n",
					elapsed, stats.ChurnSuccess.Load(), stats.ChurnFail.Load(), stats.PushReceived.Load())
			}
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

		_, err := client.namingClient.DeregisterInstance(vo.DeregisterInstanceParam{
			Ip:          client.instance.IP,
			Port:        client.instance.Port,
			ServiceName: client.instance.ServiceName,
			Ephemeral:   true,
		})

		if err != nil {
			stats.ChurnFail.Add(1)
			fmt.Printf("[Error] Churn deregister failed for service %s: %v\n", client.instance.ServiceName, err)
			continue
		}

		time.Sleep(1 * time.Second)

		_, err = client.namingClient.RegisterInstance(vo.RegisterInstanceParam{
			Ip:          client.instance.IP,
			Port:        client.instance.Port,
			ServiceName: client.instance.ServiceName,
			GroupName:   "DEFAULT_GROUP",
			ClusterName: "DEFAULT",
			Weight:      1.0,
			Enable:      true,
			Healthy:     true,
			Metadata:    client.instance.Metadata,
			Ephemeral:   true,
		})

		if err != nil {
			stats.ChurnFail.Add(1)
			fmt.Printf("[Error] Churn register failed for service %s: %v\n", client.instance.ServiceName, err)
		} else {
			stats.ChurnSuccess.Add(1)
		}

		if config.ConfigCount > 0 {
			churnConfig(client, config)
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
			_, err := c.namingClient.DeregisterInstance(vo.DeregisterInstanceParam{
				Ip:          c.instance.IP,
				Port:        c.instance.Port,
				ServiceName: c.instance.ServiceName,
				Ephemeral:   true,
			})
			if err != nil {
				stats.DeregisterFail.Add(1)
				fmt.Printf("[Error] Shutdown deregister failed for service %s: %v\n", c.instance.ServiceName, err)
			} else {
				stats.DeregisterSuccess.Add(1)
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
	if stats.ConfigPublishSuccess.Load() > 0 || stats.ConfigListenSuccess.Load() > 0 {
		fmt.Printf("  Config Publish Success: %d, Fail: %d\n", stats.ConfigPublishSuccess.Load(), stats.ConfigPublishFail.Load())
		fmt.Printf("  Config Listen Success: %d, Fail: %d\n", stats.ConfigListenSuccess.Load(), stats.ConfigListenFail.Load())
		fmt.Printf("  Config Push Received: %d\n", stats.ConfigPushReceived.Load())
	}
}
