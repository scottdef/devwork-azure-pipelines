//go:build linux || darwin

// Command vkprobe calls the system Vulkan loader from Go with cgo disabled.
// It is the smallest demonstration of how lm9-gpu reaches a GPU: open the
// vendor's library at run time, bind a few C functions to Go function
// variables, and pass C-layout structs by pointer.
//
//	go mod tidy && CGO_ENABLED=0 go run .
//
// NOT BUILT OR RUN in the session that produced this spike (see go.mod).
package main

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// instanceCreateInfo mirrors VkInstanceCreateInfo. Go lays out these field
// types the same way C does on 64-bit targets, including the padding after
// each uint32 that precedes a pointer.
type instanceCreateInfo struct {
	sType                   uint32 // VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO = 1
	pNext                   unsafe.Pointer
	flags                   uint32
	pApplicationInfo        unsafe.Pointer
	enabledLayerCount       uint32
	ppEnabledLayerNames     unsafe.Pointer
	enabledExtensionCount   uint32
	ppEnabledExtensionNames unsafe.Pointer
}

var (
	enumerateInstanceVersion func(version *uint32) int32
	createInstance           func(info *instanceCreateInfo, allocator unsafe.Pointer, instance *uintptr) int32
	enumeratePhysicalDevices func(instance uintptr, count *uint32, devices *uintptr) int32
	destroyInstance          func(instance uintptr, allocator unsafe.Pointer)
)

func main() {
	name := "libvulkan.so.1"
	if runtime.GOOS == "darwin" {
		name = "libvulkan.1.dylib" // MoltenVK or the Vulkan SDK loader
	}
	lib, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "no Vulkan loader on this machine:", err)
		os.Exit(1)
	}
	purego.RegisterLibFunc(&enumerateInstanceVersion, lib, "vkEnumerateInstanceVersion")
	purego.RegisterLibFunc(&createInstance, lib, "vkCreateInstance")
	purego.RegisterLibFunc(&enumeratePhysicalDevices, lib, "vkEnumeratePhysicalDevices")
	purego.RegisterLibFunc(&destroyInstance, lib, "vkDestroyInstance")

	var v uint32
	if r := enumerateInstanceVersion(&v); r != 0 {
		fmt.Fprintln(os.Stderr, "vkEnumerateInstanceVersion failed:", r)
		os.Exit(1)
	}
	fmt.Printf("Vulkan loader %d.%d.%d, called with cgo off\n", v>>22&0x7f, v>>12&0x3ff, v&0xfff)

	info := instanceCreateInfo{sType: 1}
	var inst uintptr
	if r := createInstance(&info, nil, &inst); r != 0 {
		// -9 is VK_ERROR_INCOMPATIBLE_DRIVER: a loader with no GPU driver installed.
		fmt.Printf("vkCreateInstance returned %d: no usable driver here\n", r)
		return
	}
	defer destroyInstance(inst, nil)
	var n uint32
	enumeratePhysicalDevices(inst, &n, nil)
	fmt.Printf("physical devices: %d\n", n)
}
