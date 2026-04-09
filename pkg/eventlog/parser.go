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

package eventlog

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

// EventXML represents the parsed structure of a Windows Event Log XML record.
type EventXML struct {
	System SystemXML `xml:"System"`
	Data   []DataXML `xml:"EventData>Data"`
}

// SystemXML contains the system-level metadata from a Windows Event Log record.
type SystemXML struct {
	Provider    ProviderXML    `xml:"Provider"`
	EventID     uint16         `xml:"EventID"`
	Version     uint8          `xml:"Version"`
	Level       uint8          `xml:"Level"`
	Task        uint16         `xml:"Task"`
	Opcode      uint8          `xml:"Opcode"`
	Keywords    string         `xml:"Keywords"`
	TimeCreated TimeCreatedXML `xml:"TimeCreated"`
	EventRecordID uint64       `xml:"EventRecordID"`
	Execution   ExecutionXML   `xml:"Execution"`
	Channel     string         `xml:"Channel"`
	Computer    string         `xml:"Computer"`
	Security    SecurityXML    `xml:"Security"`
}

// ProviderXML represents the event provider.
type ProviderXML struct {
	Name string `xml:"Name,attr"`
	GUID string `xml:"Guid,attr"`
}

// TimeCreatedXML represents the event timestamp.
type TimeCreatedXML struct {
	SystemTime string `xml:"SystemTime,attr"`
}

// ExecutionXML contains the process and thread IDs.
type ExecutionXML struct {
	ProcessID uint32 `xml:"ProcessID,attr"`
	ThreadID  uint32 `xml:"ThreadID,attr"`
}

// SecurityXML contains the security context.
type SecurityXML struct {
	UserID string `xml:"UserID,attr"`
}

// DataXML represents a single data element from EventData.
type DataXML struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:",chardata"`
}

// ParseEventXML parses a Windows Event Log XML string into an EventXML struct.
func ParseEventXML(xmlStr string) (*EventXML, error) {
	var evt EventXML
	if err := xml.Unmarshal([]byte(xmlStr), &evt); err != nil {
		return nil, err
	}
	return &evt, nil
}

// Timestamp parses the SystemTime attribute into a Go time.Time.
func (e *EventXML) Timestamp() time.Time {
	t, err := time.Parse(time.RFC3339Nano, e.System.TimeCreated.SystemTime)
	if err != nil {
		// Windows sometimes uses a different format
		t, err = time.Parse("2006-01-02T15:04:05.0000000Z", e.System.TimeCreated.SystemTime)
		if err != nil {
			return time.Now()
		}
	}
	return t
}

// DataMap returns the EventData fields as a map for easy access.
func (e *EventXML) DataMap() map[string]string {
	m := make(map[string]string, len(e.Data))
	for _, d := range e.Data {
		if d.Name != "" {
			m[d.Name] = d.Value
		}
	}
	return m
}

// LevelName returns the human-readable level name.
func (e *EventXML) LevelName() string {
	switch e.System.Level {
	case 0:
		return "LogAlways"
	case 1:
		return "Critical"
	case 2:
		return "Error"
	case 3:
		return "Warning"
	case 4:
		return "Information"
	case 5:
		return "Verbose"
	default:
		return "Level(" + strconv.Itoa(int(e.System.Level)) + ")"
	}
}

// BuildXPathQuery constructs an XPath query that filters for specific event IDs.
// If eventIDs is empty, returns "*" to match all events.
func BuildXPathQuery(eventIDs []uint16) string {
	if len(eventIDs) == 0 {
		return "*"
	}
	ids := make([]string, len(eventIDs))
	for i, id := range eventIDs {
		ids[i] = strconv.Itoa(int(id))
	}
	return "*[System[(EventID=" + strings.Join(ids, " or EventID=") + ")]]"
}
