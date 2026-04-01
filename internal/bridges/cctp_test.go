package bridges

import "testing"

func TestRegisterCCTPTestnetChains_MessageTransmitterAddresses(t *testing.T) {
	registerCCTPTestnetChains()

	const sharedTestnetMessageTransmitter = "0x7865fAfC2db2093669d92c0F33AeEF291086BEFD"
	if got := cctpMessageTransmitter[ChainIDSepolia]; got != sharedTestnetMessageTransmitter {
		t.Fatalf("sepolia message transmitter = %s, want %s", got, sharedTestnetMessageTransmitter)
	}
	if got := cctpMessageTransmitter[ChainIDBaseSepolia]; got != sharedTestnetMessageTransmitter {
		t.Fatalf("base sepolia message transmitter = %s, want %s", got, sharedTestnetMessageTransmitter)
	}
	if got := cctpMessageTransmitter[ChainIDOPSepolia]; got != sharedTestnetMessageTransmitter {
		t.Fatalf("op sepolia message transmitter = %s, want %s", got, sharedTestnetMessageTransmitter)
	}

	const arbitrumSepoliaMessageTransmitter = "0xaCF1ceeF35caAc005e15888dDb8A3515C41B4872"
	if got := cctpMessageTransmitter[ChainIDArbitrumSepolia]; got != arbitrumSepoliaMessageTransmitter {
		t.Fatalf("arbitrum sepolia message transmitter = %s, want %s", got, arbitrumSepoliaMessageTransmitter)
	}
}
