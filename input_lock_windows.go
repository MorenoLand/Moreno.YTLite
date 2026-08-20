//go:build windows

package main

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

type keyboardHookStruct struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

var (
	inputLocked atomic.Bool
	inputHook   uintptr
	user32      = syscall.NewLazyDLL("user32.dll")
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	setHook     = user32.NewProc("SetWindowsHookExW")
	callNext    = user32.NewProc("CallNextHookEx")
	getAsyncKey = user32.NewProc("GetAsyncKeyState")
	getModule   = kernel32.NewProc("GetModuleHandleW")
	copyMemory  = kernel32.NewProc("RtlMoveMemory")
)

func setInputLocked(locked bool) { inputLocked.Store(locked) }

//go:nocheckptr
func inputLockHook(code int, message uintptr, data uintptr) uintptr {
	if code == 0 && inputLocked.Load() {
		var keyboard keyboardHookStruct
		copyMemory.Call(uintptr(unsafe.Pointer(&keyboard)), data, unsafe.Sizeof(keyboard))
		key := keyboard.VKCode
		shiftDown, _, _ := getAsyncKey.Call(uintptr(0x10))
		if key != 0x10 && !(key == 0x58 && int16(shiftDown) < 0) {
			return 1
		}
	}
	result, _, _ := callNext.Call(0, uintptr(code), message, data)
	return result
}

func installInputLockHook() error {
	callback := syscall.NewCallback(inputLockHook)
	module, _, _ := getModule.Call(0)
	hook, _, err := setHook.Call(uintptr(13), callback, module, 0)
	if hook == 0 {
		return fmt.Errorf("could not install the Windows-key input lock: %w", err)
	}
	inputHook = hook
	return nil
}

func uninstallInputLockHook() {
	if inputHook != 0 {
		user32.NewProc("UnhookWindowsHookEx").Call(inputHook)
		inputHook = 0
	}
}
