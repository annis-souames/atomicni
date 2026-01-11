package main

import (
	"fmt"

	"github.com/annis-souames/atomicni/cmd"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
)

const CNI_VERSION = "1.1.0"

func main() {
	fmt.Println("Starting CNI plugin operations")

	funcs := skel.CNIFuncs{
		Add:   cmd.Add,
		Del:   cmd.Del,
		Check: cmd.Check,
	}
	// Method from CNI skel pkg that registers Add, Check, Del functions and provide info about CNI
	skel.PluginMainFuncs(
		funcs,
		version.VersionsStartingFrom(CNI_VERSION),
		"Atomic CNI Plugin - Simple CNI for learning purposes",
	)

}
