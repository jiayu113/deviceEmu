package device

import (
	"sync"
	"testing"
	"time"
)

func TestInjectHasSweep(t *testing.T) {
	f := newFaultState()
	if !f.inject(FaultCPUSpike, 50*time.Millisecond) {
		t.Fatal("first inject should be newly active")
	}
	if f.inject(FaultCPUSpike, 50*time.Millisecond) {
		t.Fatal("re-inject while active should NOT be newly active")
	}
	if !f.has(FaultCPUSpike) {
		t.Fatal("should be active right after inject")
	}
	time.Sleep(70 * time.Millisecond)
	if f.has(FaultCPUSpike) {
		t.Fatal("should have expired")
	}
	exp := f.sweep()

	if len(exp) != 1 || exp[0] != FaultCPUSpike {
		t.Fatalf("sweep want [cpu_spike], got %v", exp)
	}
}

func TestConcurrentInject(t *testing.T) {
	f := newFaultState()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.inject(FaultLatency, time.Second) }()
	}
	wg.Wait()
	if !f.has(FaultLatency) {
		t.Fatal("should be active")
	}
}
