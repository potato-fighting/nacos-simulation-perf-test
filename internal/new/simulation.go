package new

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func RunSimulation(config SimulationConfig) {
	InitSimulation(config)
	RunRegistration(config)

	if config.PerfApi == "largeScaleStable" {
		RunStablePhase(config)
	} else if config.PerfApi == "largeScaleWithChurn" {
		RunStablePhase(config)
		RunChurnPhase(config)
	}

	setupSignalHandler(config)
	select {}
}

func setupSignalHandler(config SimulationConfig) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n[Signal] Received shutdown signal")
		RunShutdown(config)
		os.Exit(0)
	}()
}
