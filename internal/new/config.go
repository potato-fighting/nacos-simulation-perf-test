package new

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

var configDataIds []string

func generateConfigDataId(machineId, configIndex int) string {
	return fmt.Sprintf("nacos.sim.config.%d.%d", machineId, configIndex)
}

func generateConfigContent(length int) string {
	return strings.Repeat("x", length)
}

func initConfigs(config SimulationConfig) {
	if config.ConfigCount == 0 {
		return
	}

	fmt.Printf("[Init] Creating %d configs...\n", config.ConfigCount)

	if len(simClients) == 0 {
		return
	}

	client := simClients[0].configClient
	for i := 0; i < config.ConfigCount; i++ {
		dataId := generateConfigDataId(config.MachineId, i)
		configDataIds = append(configDataIds, dataId)

		_, err := client.PublishConfig(vo.ConfigParam{
			DataId:  dataId,
			Group:   "DEFAULT_GROUP",
			Content: generateConfigContent(config.ConfigContentLength),
		})

		if err != nil {
			stats.ConfigPublishFail.Add(1)
			fmt.Printf("[Error] Failed to publish config %s: %v\n", dataId, err)
		} else {
			stats.ConfigPublishSuccess.Add(1)
		}
	}

	fmt.Printf("[Init] Config publish success: %d, fail: %d\n",
		stats.ConfigPublishSuccess.Load(), stats.ConfigPublishFail.Load())
}

func listenConfigs(client *SimulationClient, config SimulationConfig) {
	if config.ConfigCount == 0 || config.ConfigListenPerClient == 0 {
		return
	}

	rnd := rand.New(rand.NewSource(int64(client.clientId)))
	listenedSet := make(map[int]bool)

	for i := 0; i < config.ConfigListenPerClient && i < config.ConfigCount; i++ {
		var randomConfigIdx int
		for {
			randomConfigIdx = rnd.Intn(config.ConfigCount)
			if !listenedSet[randomConfigIdx] {
				listenedSet[randomConfigIdx] = true
				break
			}
		}

		dataId := generateConfigDataId(config.MachineId, randomConfigIdx)

		if config.Debug {
			fmt.Printf("[ConfigListen] Client %d listening to config: %s\n", client.clientId, dataId)
		}

		err := client.configClient.ListenConfig(vo.ConfigParam{
			DataId: dataId,
			Group:  "DEFAULT_GROUP",
			OnChange: func(namespace, group, dataId, data string) {
				stats.ConfigPushReceived.Add(1)
				if config.Debug {
					fmt.Printf("[ConfigPush] Client %d received config change: %s\n", client.clientId, dataId)
				}
			},
		})

		if err != nil {
			stats.ConfigListenFail.Add(1)
			fmt.Printf("[Error] Config listen failed for %s: %v\n", dataId, err)
		} else {
			stats.ConfigListenSuccess.Add(1)
			client.listenedConfigs = append(client.listenedConfigs, dataId)
		}
	}
}

func churnConfig(client *SimulationClient, config SimulationConfig) {
	if config.ConfigCount == 0 {
		return
	}

	rnd := rand.New(rand.NewSource(int64(client.clientId)))
	configIdx := rnd.Intn(config.ConfigCount)
	dataId := generateConfigDataId(config.MachineId, configIdx)

	_, err := client.configClient.PublishConfig(vo.ConfigParam{
		DataId:  dataId,
		Group:   "DEFAULT_GROUP",
		Content: generateConfigContent(config.ConfigContentLength) + fmt.Sprintf(".%d", time.Now().Unix()),
	})

	if err != nil {
		stats.ConfigPublishFail.Add(1)
	} else {
		stats.ConfigPublishSuccess.Add(1)
	}
}
