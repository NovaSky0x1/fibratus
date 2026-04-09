/*
 * Copyright 2021-2022 by Nedim Sabic Sabic
 * https://www.fibratus.io
 * All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tamper

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// WFP GUIDs — unique to Fibratus. An attacker must know these exact GUIDs
// AND have SYSTEM access to remove the filters.
var (
	fibratusProviderKey = windows.GUID{
		Data1: 0xa1b2c3d4, Data2: 0xe5f6, Data3: 0x7890,
		Data4: [8]byte{0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89},
	}
	fibratusSubLayerKey = windows.GUID{
		Data1: 0xf1e2d3c4, Data2: 0xb5a6, Data3: 0x9780,
		Data4: [8]byte{0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10},
	}
	// Filter keys for each block/permit rule — needed for deletion
	filterBlockOutbound = windows.GUID{
		Data1: 0x11111111, Data2: 0x1111, Data3: 0x1111,
		Data4: [8]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x01},
	}
	filterBlockInbound = windows.GUID{
		Data1: 0x11111111, Data2: 0x1111, Data3: 0x1111,
		Data4: [8]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x02},
	}
)

// WFP API constants
const (
	fwpmFilterFlagPersistent = 0x00000001
	fwpmFilterActionBlock    = 0x00000001 // FWP_ACTION_BLOCK
	fwpmFilterActionPermit   = 0x00001000 // FWP_ACTION_PERMIT

	// Layer GUIDs (from fwpmu.h)
	// FWPM_LAYER_ALE_AUTH_CONNECT_V4
	// FWPM_LAYER_ALE_AUTH_RECV_ACCEPT_V4
)

var (
	// ALE Auth Connect V4 layer — outbound connections
	layerALEAuthConnectV4 = windows.GUID{
		Data1: 0xc38d57d1, Data2: 0x05a7, Data3: 0x4c33,
		Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82},
	}
	// ALE Auth Recv Accept V4 layer — inbound connections
	layerALEAuthRecvAcceptV4 = windows.GUID{
		Data1: 0xe1cd9fe7, Data2: 0xf4b5, Data3: 0x4273,
		Data4: [8]byte{0x96, 0xc0, 0x59, 0x2e, 0x48, 0x7b, 0x86, 0x50},
	}
	// Condition field: IP remote address
	condFieldIPRemoteAddress = windows.GUID{
		Data1: 0xb235ae9a, Data2: 0x1d64, Data3: 0x49b8,
		Data4: [8]byte{0xa4, 0x4c, 0x5f, 0xf3, 0xd9, 0x09, 0x50, 0x45},
	}
	// Condition field: IP remote port
	condFieldIPRemotePort = windows.GUID{
		Data1: 0xc35a604d, Data2: 0xd22b, Data3: 0x4e1a,
		Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b},
	}
	// Condition field: IP protocol
	condFieldIPProtocol = windows.GUID{
		Data1: 0x3971ef2b, Data2: 0x623e, Data3: 0x4f9a,
		Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7},
	}
	// Condition field: IP local address
	condFieldIPLocalAddress = windows.GUID{
		Data1: 0xd9ee00de, Data2: 0xc1ef, Data3: 0x4617,
		Data4: [8]byte{0xbf, 0xe3, 0xff, 0xd8, 0xf5, 0xa0, 0x89, 0x57},
	}
)

// WFP API DLL bindings
var (
	fwpuclnt                   = windows.NewLazyDLL("fwpuclnt.dll")
	procFwpmEngineOpen0        = fwpuclnt.NewProc("FwpmEngineOpen0")
	procFwpmEngineClose0       = fwpuclnt.NewProc("FwpmEngineClose0")
	procFwpmTransactionBegin0  = fwpuclnt.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit0 = fwpuclnt.NewProc("FwpmTransactionCommit0")
	procFwpmTransactionAbort0  = fwpuclnt.NewProc("FwpmTransactionAbort0")
	procFwpmProviderAdd0       = fwpuclnt.NewProc("FwpmProviderAdd0")
	procFwpmSubLayerAdd0       = fwpuclnt.NewProc("FwpmSubLayerAdd0")
	procFwpmFilterAdd0         = fwpuclnt.NewProc("FwpmFilterAdd0")
	procFwpmFilterDeleteByKey0 = fwpuclnt.NewProc("FwpmFilterDeleteByKey0")
	procFwpmProviderDeleteByKey0 = fwpuclnt.NewProc("FwpmProviderDeleteByKey0")
	procFwpmSubLayerDeleteByKey0 = fwpuclnt.NewProc("FwpmSubLayerDeleteByKey0")
)

// WFP C structures (simplified for our use case)

type fwpmDisplayData0 struct {
	name        *uint16
	description *uint16
}

type fwpmProvider0 struct {
	providerKey  windows.GUID
	displayData  fwpmDisplayData0
	flags        uint32
	providerData fwpByteBlob
	serviceName  *uint16
}

type fwpmSubLayer0 struct {
	subLayerKey windows.GUID
	displayData fwpmDisplayData0
	flags       uint32
	providerKey *windows.GUID
	weight      fwpValue0
	reserved    uint32
}

type fwpmFilter0 struct {
	filterKey       windows.GUID
	displayData     fwpmDisplayData0
	flags           uint32
	providerKey     *windows.GUID
	providerData    fwpByteBlob
	layerKey        windows.GUID
	subLayerKey     windows.GUID
	weight          fwpValue0
	numFilterConds  uint32
	filterCondition *fwpmFilterCondition0
	action          fwpmAction0
	_               [16]byte // union context/rawContext/providerContextKey
	reserved        *windows.GUID
	filterId        uint64
	effectiveWeight fwpValue0
}

type fwpmAction0 struct {
	actionType uint32
	_          [16]byte // union filterType/calloutKey
}

type fwpmFilterCondition0 struct {
	fieldKey       windows.GUID
	matchType      uint32
	conditionValue fwpConditionValue0
}

type fwpConditionValue0 struct {
	valueType uint32
	value     uintptr
}

type fwpValue0 struct {
	valueType uint32
	value     uintptr
}

type fwpByteBlob struct {
	size uint32
	data *byte
}

// Match types
const (
	fwpMatchEqual   = 0
	fwpMatchGreater = 3
)

// Value types
const (
	fwpUint8    = 0
	fwpUint16   = 1
	fwpUint32   = 2
	fwpByteArr4 = 11 // FWP_BYTE_ARRAY16_TYPE for IPv4 is actually uint32
	fwpV4AddrMask = 13
)

// WFPIsolator manages network isolation via WFP kernel filters.
type WFPIsolator struct {
	mu            sync.Mutex
	isolated      bool
	filterKeys    []windows.GUID // track added permit filter keys for cleanup
	permitCounter uint32
}

// NewWFPIsolator creates a new WFP-based network isolator.
func NewWFPIsolator() *WFPIsolator {
	return &WFPIsolator{}
}

// Isolate blocks all network traffic except for the specified whitelist IPs.
// Whitelist should include the fleet server IP at minimum.
func (w *WFPIsolator) Isolate(serverIP string, whitelistIPs []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.isolated {
		return nil // already isolated
	}

	engine, err := openEngine()
	if err != nil {
		return fmt.Errorf("wfp: open engine: %w", err)
	}
	defer closeEngine(engine)

	if err := beginTransaction(engine); err != nil {
		return fmt.Errorf("wfp: begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			abortTransaction(engine)
		}
	}()

	// Register provider (idempotent — ignore already-exists error)
	if err := addProvider(engine); err != nil {
		log.Debugf("wfp: provider add (may already exist): %v", err)
	}

	// Register sublayer (idempotent)
	if err := addSubLayer(engine); err != nil {
		log.Debugf("wfp: sublayer add (may already exist): %v", err)
	}

	// Add block-all filters
	if err := addBlockFilter(engine, filterBlockOutbound, layerALEAuthConnectV4, "Fibratus Isolation - Block Outbound"); err != nil {
		return fmt.Errorf("wfp: add block outbound: %w", err)
	}
	if err := addBlockFilter(engine, filterBlockInbound, layerALEAuthRecvAcceptV4, "Fibratus Isolation - Block Inbound"); err != nil {
		return fmt.Errorf("wfp: add block inbound: %w", err)
	}

	// Add permit filters for whitelist
	w.filterKeys = nil
	w.permitCounter = 0

	// Always permit: fleet server
	if serverIP != "" {
		if ip := net.ParseIP(serverIP); ip != nil {
			if key, err := w.addPermitIP(engine, ip); err == nil {
				w.filterKeys = append(w.filterKeys, key)
			}
		}
	}

	// Always permit: DNS (UDP 53)
	if key, err := w.addPermitDNS(engine); err == nil {
		w.filterKeys = append(w.filterKeys, key)
	}

	// Always permit: loopback
	if key, err := w.addPermitLoopback(engine); err == nil {
		w.filterKeys = append(w.filterKeys, key)
	}

	// Whitelist IPs/CIDRs
	for _, addr := range whitelistIPs {
		addr := parseAddr(addr)
		if addr == "" {
			continue
		}
		if ip := net.ParseIP(addr); ip != nil {
			if key, err := w.addPermitIP(engine, ip); err == nil {
				w.filterKeys = append(w.filterKeys, key)
			}
		}
	}

	if err := commitTransaction(engine); err != nil {
		return fmt.Errorf("wfp: commit: %w", err)
	}
	committed = true
	w.isolated = true

	log.Infof("wfp: network isolated — %d permit filters, server=%s, whitelist=%d IPs",
		len(w.filterKeys), serverIP, len(whitelistIPs))
	return nil
}

// Unisolate removes all WFP isolation filters.
func (w *WFPIsolator) Unisolate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isolated {
		return nil
	}

	engine, err := openEngine()
	if err != nil {
		return fmt.Errorf("wfp: open engine: %w", err)
	}
	defer closeEngine(engine)

	if err := beginTransaction(engine); err != nil {
		return fmt.Errorf("wfp: begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			abortTransaction(engine)
		}
	}()

	// Remove block filters
	deleteFilter(engine, filterBlockOutbound)
	deleteFilter(engine, filterBlockInbound)

	// Remove permit filters
	for _, key := range w.filterKeys {
		deleteFilter(engine, key)
	}

	if err := commitTransaction(engine); err != nil {
		return fmt.Errorf("wfp: commit: %w", err)
	}
	committed = true
	w.isolated = false
	w.filterKeys = nil

	log.Info("wfp: network isolation removed")
	return nil
}

// IsIsolated returns whether the network is currently isolated.
func (w *WFPIsolator) IsIsolated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.isolated
}

// Cleanup removes all Fibratus WFP objects (provider, sublayer, filters).
func (w *WFPIsolator) Cleanup() error {
	w.Unisolate()

	engine, err := openEngine()
	if err != nil {
		return err
	}
	defer closeEngine(engine)

	procFwpmSubLayerDeleteByKey0.Call(engine, uintptr(unsafe.Pointer(&fibratusSubLayerKey)))
	procFwpmProviderDeleteByKey0.Call(engine, uintptr(unsafe.Pointer(&fibratusProviderKey)))
	return nil
}

// nextPermitKey generates a unique GUID for each permit filter.
func (w *WFPIsolator) nextPermitKey() windows.GUID {
	w.permitCounter++
	return windows.GUID{
		Data1: 0x22222222, Data2: 0x2222, Data3: 0x2222,
		Data4: [8]byte{0x22, 0x22, 0x22, 0x22, 0x22, 0x22, byte(w.permitCounter >> 8), byte(w.permitCounter)},
	}
}

// ── WFP Engine operations ────────────────────────────────

func openEngine() (uintptr, error) {
	var engine uintptr
	ret, _, _ := procFwpmEngineOpen0.Call(
		0, // serverName (local)
		0, // authnService (RPC_C_AUTHN_DEFAULT)
		0, // authIdentity
		0, // session
		uintptr(unsafe.Pointer(&engine)),
	)
	if ret != 0 {
		return 0, fmt.Errorf("FwpmEngineOpen0: 0x%x", ret)
	}
	return engine, nil
}

func closeEngine(engine uintptr) {
	procFwpmEngineClose0.Call(engine)
}

func beginTransaction(engine uintptr) error {
	ret, _, _ := procFwpmTransactionBegin0.Call(engine, 0)
	if ret != 0 {
		return fmt.Errorf("FwpmTransactionBegin0: 0x%x", ret)
	}
	return nil
}

func commitTransaction(engine uintptr) error {
	ret, _, _ := procFwpmTransactionCommit0.Call(engine)
	if ret != 0 {
		return fmt.Errorf("FwpmTransactionCommit0: 0x%x", ret)
	}
	return nil
}

func abortTransaction(engine uintptr) {
	procFwpmTransactionAbort0.Call(engine)
}

func addProvider(engine uintptr) error {
	name, _ := windows.UTF16PtrFromString("Fibratus EDR")
	desc, _ := windows.UTF16PtrFromString("Fibratus Endpoint Detection and Response")

	provider := fwpmProvider0{
		providerKey: fibratusProviderKey,
		displayData: fwpmDisplayData0{name: name, description: desc},
		flags:       0x00000001, // FWPM_PROVIDER_FLAG_PERSISTENT
	}

	ret, _, _ := procFwpmProviderAdd0.Call(engine, uintptr(unsafe.Pointer(&provider)), 0)
	if ret != 0 && ret != 0x80320009 { // FWP_E_ALREADY_EXISTS
		return fmt.Errorf("FwpmProviderAdd0: 0x%x", ret)
	}
	return nil
}

func addSubLayer(engine uintptr) error {
	name, _ := windows.UTF16PtrFromString("Fibratus Isolation")
	desc, _ := windows.UTF16PtrFromString("Network isolation sublayer for Fibratus EDR")

	subLayer := fwpmSubLayer0{
		subLayerKey: fibratusSubLayerKey,
		displayData: fwpmDisplayData0{name: name, description: desc},
		flags:       0x00000001, // FWPM_SUBLAYER_FLAG_PERSISTENT
		providerKey: &fibratusProviderKey,
		weight:      fwpValue0{valueType: fwpUint16, value: 0xFFFF}, // highest weight
	}

	ret, _, _ := procFwpmSubLayerAdd0.Call(engine, uintptr(unsafe.Pointer(&subLayer)), 0)
	if ret != 0 && ret != 0x80320009 { // FWP_E_ALREADY_EXISTS
		return fmt.Errorf("FwpmSubLayerAdd0: 0x%x", ret)
	}
	return nil
}

func addBlockFilter(engine uintptr, key, layerKey windows.GUID, name string) error {
	namePtr, _ := windows.UTF16PtrFromString(name)

	filter := fwpmFilter0{
		filterKey:   key,
		displayData: fwpmDisplayData0{name: namePtr},
		flags:       fwpmFilterFlagPersistent,
		providerKey: &fibratusProviderKey,
		layerKey:    layerKey,
		subLayerKey: fibratusSubLayerKey,
		weight:      fwpValue0{valueType: fwpUint8, value: 1}, // low weight — permits override
		action:      fwpmAction0{actionType: fwpmFilterActionBlock},
	}

	var filterId uint64
	ret, _, _ := procFwpmFilterAdd0.Call(engine, uintptr(unsafe.Pointer(&filter)), 0, uintptr(unsafe.Pointer(&filterId)))
	if ret != 0 && ret != 0x80320009 {
		return fmt.Errorf("FwpmFilterAdd0 (block): 0x%x", ret)
	}
	return nil
}

func (w *WFPIsolator) addPermitIP(engine uintptr, ip net.IP) (windows.GUID, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return windows.GUID{}, fmt.Errorf("only IPv4 supported")
	}

	key := w.nextPermitKey()
	namePtr, _ := windows.UTF16PtrFromString(fmt.Sprintf("Fibratus Permit %s", ip.String()))

	// IP as uint32 in network byte order
	ipUint32 := binary.BigEndian.Uint32(ip4)

	cond := fwpmFilterCondition0{
		fieldKey:  condFieldIPRemoteAddress,
		matchType: fwpMatchEqual,
		conditionValue: fwpConditionValue0{
			valueType: fwpUint32,
			value:     uintptr(ipUint32),
		},
	}

	filter := fwpmFilter0{
		filterKey:       key,
		displayData:     fwpmDisplayData0{name: namePtr},
		flags:           fwpmFilterFlagPersistent,
		providerKey:     &fibratusProviderKey,
		layerKey:        layerALEAuthConnectV4, // outbound
		subLayerKey:     fibratusSubLayerKey,
		weight:          fwpValue0{valueType: fwpUint8, value: 10}, // higher than block
		numFilterConds:  1,
		filterCondition: &cond,
		action:          fwpmAction0{actionType: fwpmFilterActionPermit},
	}

	var filterId uint64
	ret, _, _ := procFwpmFilterAdd0.Call(engine, uintptr(unsafe.Pointer(&filter)), 0, uintptr(unsafe.Pointer(&filterId)))
	if ret != 0 && ret != 0x80320009 {
		return key, fmt.Errorf("FwpmFilterAdd0 (permit %s): 0x%x", ip.String(), ret)
	}

	// Also add inbound permit for this IP
	inKey := w.nextPermitKey()
	filter.filterKey = inKey
	filter.layerKey = layerALEAuthRecvAcceptV4
	procFwpmFilterAdd0.Call(engine, uintptr(unsafe.Pointer(&filter)), 0, uintptr(unsafe.Pointer(&filterId)))
	w.filterKeys = append(w.filterKeys, inKey)

	return key, nil
}

func (w *WFPIsolator) addPermitDNS(engine uintptr) (windows.GUID, error) {
	key := w.nextPermitKey()
	namePtr, _ := windows.UTF16PtrFromString("Fibratus Permit DNS")

	// Condition: remote port == 53
	portCond := fwpmFilterCondition0{
		fieldKey:  condFieldIPRemotePort,
		matchType: fwpMatchEqual,
		conditionValue: fwpConditionValue0{
			valueType: fwpUint16,
			value:     53,
		},
	}

	// Condition: protocol == UDP (17)
	protoCond := fwpmFilterCondition0{
		fieldKey:  condFieldIPProtocol,
		matchType: fwpMatchEqual,
		conditionValue: fwpConditionValue0{
			valueType: fwpUint8,
			value:     17, // UDP
		},
	}

	conditions := [2]fwpmFilterCondition0{portCond, protoCond}

	filter := fwpmFilter0{
		filterKey:       key,
		displayData:     fwpmDisplayData0{name: namePtr},
		flags:           fwpmFilterFlagPersistent,
		providerKey:     &fibratusProviderKey,
		layerKey:        layerALEAuthConnectV4,
		subLayerKey:     fibratusSubLayerKey,
		weight:          fwpValue0{valueType: fwpUint8, value: 10},
		numFilterConds:  2,
		filterCondition: &conditions[0],
		action:          fwpmAction0{actionType: fwpmFilterActionPermit},
	}

	var filterId uint64
	ret, _, _ := procFwpmFilterAdd0.Call(engine, uintptr(unsafe.Pointer(&filter)), 0, uintptr(unsafe.Pointer(&filterId)))
	if ret != 0 && ret != 0x80320009 {
		return key, fmt.Errorf("FwpmFilterAdd0 (DNS): 0x%x", ret)
	}
	return key, nil
}

func (w *WFPIsolator) addPermitLoopback(engine uintptr) (windows.GUID, error) {
	return w.addPermitIP(engine, net.ParseIP("127.0.0.1"))
}

func deleteFilter(engine uintptr, key windows.GUID) {
	ret, _, _ := procFwpmFilterDeleteByKey0.Call(engine, uintptr(unsafe.Pointer(&key)))
	if ret != 0 {
		log.Warnf("wfp: failed to delete filter %v: 0x%x", key, ret)
	}
}

func parseAddr(s string) string {
	// Direct IP
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	// Try CIDR — extract the IP
	if ip, _, err := net.ParseCIDR(s); err == nil {
		return ip.String()
	}
	// Try hostname resolution
	ips, err := net.LookupIP(s)
	if err == nil {
		for _, ip := range ips {
			if ip.To4() != nil {
				return ip.String()
			}
		}
		if len(ips) > 0 {
			return ips[0].String()
		}
	}
	return ""
}
