// cmd package provides the cmdAdd, cmdDel, and cmdCheck functions for CNI plugin operations: ADD, DEL and CHECK, respectively.
//It uses the skeleton package from the CNI library to handle command execution and versioning.
//
// Refer to [CNI spec](https://www.cni.dev/docs/spec/) to better understand each command's purpose.

package cmd

import (
	"context"
	"fmt"

	"github.com/annis-souames/atomicni/pkg/atomicni"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

// Add adds a container to a network or apply modifications.
func Add(args *skel.CmdArgs) error {
	plugin := atomicni.NewPlugin()
	res, err := plugin.Add(context.Background(), args)
	if err != nil {
		return err
	}
	if err := types.PrintResult(res, res.CNIVersion); err != nil {
		return fmt.Errorf("print CNI result: %w", err)
	}
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
