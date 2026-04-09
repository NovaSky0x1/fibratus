/*
 * Copyright 2021-2026 by Nedim Sabic Sabic
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

package wevtapi

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modwevtapi = windows.NewLazySystemDLL("wevtapi.dll")

	procEvtSubscribe       = modwevtapi.NewProc("EvtSubscribe")
	procEvtNext            = modwevtapi.NewProc("EvtNext")
	procEvtRender          = modwevtapi.NewProc("EvtRender")
	procEvtClose           = modwevtapi.NewProc("EvtClose")
	procEvtCreateBookmark  = modwevtapi.NewProc("EvtCreateBookmark")
	procEvtUpdateBookmark  = modwevtapi.NewProc("EvtUpdateBookmark")
	procEvtCreateRenderContext = modwevtapi.NewProc("EvtCreateRenderContext")
)

// EVT_SUBSCRIBE_FLAGS specifies when to start subscribing to events.
type EvtSubscribeFlags uint32

const (
	// EvtSubscribeToFutureEvents subscribes to only future events.
	EvtSubscribeToFutureEvents EvtSubscribeFlags = 1
	// EvtSubscribeStartAtOldestRecord subscribes to all existing and future events.
	EvtSubscribeStartAtOldestRecord EvtSubscribeFlags = 2
	// EvtSubscribeStartAfterBookmark subscribes to events after the bookmarked event.
	EvtSubscribeStartAfterBookmark EvtSubscribeFlags = 3
)

// EVT_RENDER_FLAGS specifies what to render.
type EvtRenderFlags uint32

const (
	// EvtRenderEventValues renders specific event values identified by context.
	EvtRenderEventValues EvtRenderFlags = 0
	// EvtRenderEventXml renders the event as XML.
	EvtRenderEventXml EvtRenderFlags = 1
	// EvtRenderBookmark renders the bookmark as XML.
	EvtRenderBookmark EvtRenderFlags = 2
)

// EvtRenderContextFlags specifies which context values to render.
type EvtRenderContextFlags uint32

const (
	// EvtRenderContextValues renders specific properties.
	EvtRenderContextValues EvtRenderContextFlags = 0
	// EvtRenderContextSystem renders system properties.
	EvtRenderContextSystem EvtRenderContextFlags = 1
	// EvtRenderContextUser renders user properties.
	EvtRenderContextUser EvtRenderContextFlags = 2
)

// EvtHandle is the Windows Event Log handle type.
type EvtHandle uintptr

// Subscribe creates a subscription to events from the specified channel.
// The callback will be invoked when events matching the query become available.
// If bookmark is non-zero and flags is EvtSubscribeStartAfterBookmark, events
// after the bookmarked position are returned.
func Subscribe(
	channelPath string,
	query string,
	bookmark EvtHandle,
	signalEvent windows.Handle,
	flags EvtSubscribeFlags,
) (EvtHandle, error) {
	channel, err := syscall.UTF16PtrFromString(channelPath)
	if err != nil {
		return 0, err
	}
	var q *uint16
	if query != "" {
		q, err = syscall.UTF16PtrFromString(query)
		if err != nil {
			return 0, err
		}
	}
	r, _, e := procEvtSubscribe.Call(
		0, // Session (nil = local)
		uintptr(signalEvent),
		uintptr(unsafe.Pointer(channel)),
		uintptr(unsafe.Pointer(q)),
		uintptr(bookmark),
		0, // Context (for callback)
		0, // Callback (nil = signal event mode)
		uintptr(flags),
	)
	if r == 0 {
		return 0, e
	}
	return EvtHandle(r), nil
}

// Next retrieves the next batch of events from the subscription or query result set.
// Returns the events and the number of events returned. If no events are available,
// returns ERROR_NO_MORE_ITEMS.
func Next(resultSet EvtHandle, events []EvtHandle, timeout uint32) (uint32, error) {
	var returned uint32
	r, _, e := procEvtNext.Call(
		uintptr(resultSet),
		uintptr(len(events)),
		uintptr(unsafe.Pointer(&events[0])),
		uintptr(timeout),
		0, // Reserved
		uintptr(unsafe.Pointer(&returned)),
	)
	if r == 0 {
		return returned, e
	}
	return returned, nil
}

// RenderXML renders the event as an XML string.
func RenderXML(eventHandle EvtHandle) (string, error) {
	// First call to determine buffer size
	var bufferUsed, propertyCount uint32
	procEvtRender.Call(
		0, // Context
		uintptr(eventHandle),
		uintptr(EvtRenderEventXml),
		0,
		0,
		uintptr(unsafe.Pointer(&bufferUsed)),
		uintptr(unsafe.Pointer(&propertyCount)),
	)
	if bufferUsed == 0 {
		return "", syscall.GetLastError()
	}

	buf := make([]uint16, bufferUsed/2)
	r, _, e := procEvtRender.Call(
		0,
		uintptr(eventHandle),
		uintptr(EvtRenderEventXml),
		uintptr(bufferUsed),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufferUsed)),
		uintptr(unsafe.Pointer(&propertyCount)),
	)
	if r == 0 {
		return "", e
	}
	return syscall.UTF16ToString(buf), nil
}

// RenderBookmark renders the bookmark as an XML string for persistence.
func RenderBookmark(bookmark EvtHandle) (string, error) {
	var bufferUsed, propertyCount uint32
	procEvtRender.Call(
		0,
		uintptr(bookmark),
		uintptr(EvtRenderBookmark),
		0,
		0,
		uintptr(unsafe.Pointer(&bufferUsed)),
		uintptr(unsafe.Pointer(&propertyCount)),
	)
	if bufferUsed == 0 {
		return "", syscall.GetLastError()
	}

	buf := make([]uint16, bufferUsed/2)
	r, _, e := procEvtRender.Call(
		0,
		uintptr(bookmark),
		uintptr(EvtRenderBookmark),
		uintptr(bufferUsed),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufferUsed)),
		uintptr(unsafe.Pointer(&propertyCount)),
	)
	if r == 0 {
		return "", e
	}
	return syscall.UTF16ToString(buf), nil
}

// CreateBookmark creates a bookmark from the specified XML string.
// If xml is empty, creates an empty bookmark that can be updated later.
func CreateBookmark(xml string) (EvtHandle, error) {
	var xmlPtr *uint16
	if xml != "" {
		var err error
		xmlPtr, err = syscall.UTF16PtrFromString(xml)
		if err != nil {
			return 0, err
		}
	}
	r, _, e := procEvtCreateBookmark.Call(uintptr(unsafe.Pointer(xmlPtr)))
	if r == 0 {
		return 0, e
	}
	return EvtHandle(r), nil
}

// UpdateBookmark updates the bookmark with the specified event.
func UpdateBookmark(bookmark, event EvtHandle) error {
	r, _, e := procEvtUpdateBookmark.Call(
		uintptr(bookmark),
		uintptr(event),
	)
	if r == 0 {
		return e
	}
	return nil
}

// Close closes the specified handle.
func Close(handle EvtHandle) error {
	if handle == 0 {
		return nil
	}
	r, _, e := procEvtClose.Call(uintptr(handle))
	if r == 0 {
		return e
	}
	return nil
}
