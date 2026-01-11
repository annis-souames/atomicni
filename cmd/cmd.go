// cmd package provides the cmdAdd, cmdDel, and cmdCheck functions for CNI plugin operations: ADD, DEL and CHECK, respectively.
//It uses the skeleton package from the CNI library to handle command execution and versioning.
//
// Refer to [CNI spec](https://www.cni.dev/docs/spec/) to better understand each command's purpose.

package cmd

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"

	"github.com/containernetworking/plugins/pkg/ns"
)

// Add adds a container to a network or apply modifications.
func Add(args *skel.CmdArgs) error {
	// get the network namespace from args env NETNS
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to get netns %q: %v", args.Netns, err)
	}
	// Implementation for ADD command
	fmt.Println("Executing cmdAdd in netns:", netns.Path())
	return nil
}

// Del removes a container from a network or reverts modifications.
func Del(args *skel.CmdArgs) error {
	// Implementation for Del command
	fmt.Println("Executing cmdDel")
	return nil
}

// Check verifies the current state of a container's network configuration.
func Check(args *skel.CmdArgs) error {
	// Implementation for CHECK command
	fmt.Println("Executing cmdCheck")
	return nil
}
