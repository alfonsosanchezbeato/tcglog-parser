package tcglog

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"
)

type Format uint
type PCRIndex uint32
type EventType uint32
type AlgorithmId uint16
type Digest []byte
type DigestMap map[AlgorithmId]Digest

type InvalidLogError struct {
	s string
}

type Event struct {
	PCRIndex  PCRIndex
	EventType EventType
	Digests   DigestMap
	Data      []byte
}

type stream interface {
	ReadNextEvent() (*Event, bool, error)
}

type Log struct {
	Format     Format
	Algorithms []AlgorithmId
	stream     stream
}

var knownAlgorithms = map[AlgorithmId]uint16{
	AlgorithmSha1:   20,
	AlgorithmSha256: 32,
	AlgorithmSha384: 48,
	AlgorithmSha512: 64,
}

type nativeEndian_ struct{}

func (nativeEndian_) Uint16(b []byte) uint16 {
	_ = b[1]
	return *(*uint16)(unsafe.Pointer(&b[0]))
}

func (nativeEndian_) Uint32(b []byte) uint32 {
	_ = b[3]
	return *(*uint32)(unsafe.Pointer(&b[0]))
}

func (nativeEndian_) Uint64(b []byte) uint64 {
	_ = b[7]
	return *(*uint64)(unsafe.Pointer(&b[0]))
}

func (nativeEndian_) PutUint16(b []byte, v uint16) {
	_ = b[1]
	*(*uint16)(unsafe.Pointer(&b[0])) = v
}

func (nativeEndian_) PutUint32(b []byte, v uint32) {
	_ = b[3]
	*(*uint32)(unsafe.Pointer(&b[0])) = v
}

func (nativeEndian_) PutUint64(b []byte, v uint64) {
	_ = b[7]
	*(*uint64)(unsafe.Pointer(&b[0])) = v
}

func (nativeEndian_) String() string {
	return "nativeEndian"
}

var nativeEndian nativeEndian_

const maxPCRIndex PCRIndex = 31

type stream_1_2 struct {
	r io.ReadSeeker
}

// https://trustedcomputinggroup.org/wp-content/uploads/TCG_PCClientImplementation_1-21_1_00.pdf (section 11.1.1, "TCG_PCClientPCREventStruct Structure")
func (s *stream_1_2) ReadNextEvent() (*Event, bool, error) {
	var pcrIndex PCRIndex
	if err := binary.Read(s.r, nativeEndian, &pcrIndex); err != nil {
		return nil, false, err
	}

	if pcrIndex > maxPCRIndex {
		err := &InvalidLogError{fmt.Sprintf("Invalid PCR index '%d'", pcrIndex)}
		return nil, true, err
	}

	var eventType EventType
	if err := binary.Read(s.r, nativeEndian, &eventType); err != nil {
		return nil, true, err
	}

	digest := make(Digest, knownAlgorithms[AlgorithmSha1])
	if _, err := s.r.Read(digest); err != nil {
		return nil, true, err
	}
	digests := make(DigestMap)
	digests[AlgorithmSha1] = digest

	var eventSize uint32
	if err := binary.Read(s.r, nativeEndian, &eventSize); err != nil {
		return nil, true, err
	}

	event := make([]byte, eventSize)
	if _, err := io.ReadFull(s.r, event); err != nil {
		return nil, true, err
	}

	return &Event{
		PCRIndex:  pcrIndex,
		EventType: eventType,
		Digests:   digests,
		Data:      event,
	}, false, nil
}

type algorithmSize struct {
	algorithmId AlgorithmId
	digestSize  uint16
}

type stream_2 struct {
	r              io.ReadSeeker
	algorithmSizes []algorithmSize
	readFirstEvent bool
}

// https://trustedcomputinggroup.org/wp-content/uploads/PC-ClientSpecific_Platform_Profile_for_TPM_2p0_Systems_v51.pdf (section 9.2.2, "TCG_PCR_EVENT2 Structure")
func (s *stream_2) ReadNextEvent() (*Event, bool, error) {
	if !s.readFirstEvent {
		s.readFirstEvent = true
		stream := stream_1_2{s.r}
		return stream.ReadNextEvent()
	}

	var pcrIndex PCRIndex
	if err := binary.Read(s.r, nativeEndian, &pcrIndex); err != nil {
		return nil, false, err
	}

	if pcrIndex > maxPCRIndex {
		err := &InvalidLogError{fmt.Sprintf("Invalid PCR index '%d'", pcrIndex)}
		return nil, true, err
	}

	var eventType EventType
	if err := binary.Read(s.r, nativeEndian, &eventType); err != nil {
		return nil, true, err
	}

	var count uint32
	if err := binary.Read(s.r, nativeEndian, &count); err != nil {
		return nil, true, err
	}

	digests := make(DigestMap)

	for i := uint32(0); i < count; i++ {
		var algorithmId AlgorithmId
		if err := binary.Read(s.r, nativeEndian, &algorithmId); err != nil {
			return nil, true, err
		}

		var digestSize uint16
		var j int
		for j = 0; j < len(s.algorithmSizes); j++ {
			if s.algorithmSizes[j].algorithmId == algorithmId {
				digestSize = s.algorithmSizes[j].digestSize
				break
			}
		}

		if j == len(s.algorithmSizes) {
			err := &InvalidLogError{
				fmt.Sprintf("Entry for algorithm '%04x' not found in log header", algorithmId)}
			return nil, true, err
		}

		digest := make(Digest, digestSize)
		if _, err := io.ReadFull(s.r, digest); err != nil {
			return nil, true, err
		}

		if _, known := knownAlgorithms[algorithmId]; known {
			digests[algorithmId] = digest
		}
	}

	var eventSize uint32
	if err := binary.Read(s.r, nativeEndian, &eventSize); err != nil {
		return nil, true, err
	}

	event := make([]byte, eventSize)
	if _, err := io.ReadFull(s.r, event); err != nil {
		return nil, true, err
	}

	return &Event{
		PCRIndex:  pcrIndex,
		EventType: eventType,
		Digests:   digests,
		Data:      event,
	}, false, nil
}

func parseTCG2LogHeader(event *Event) []algorithmSize {
	if event.PCRIndex != 0 {
		return nil
	}

	if event.EventType != EventTypeNoAction {
		return nil
	}

	for _, b := range event.Digests[AlgorithmSha1] {
		if b != 0 {
			return nil
		}
	}

	if len(event.Data) < 29 {
		return nil
	}

	var signature strings.Builder
	if _, err := signature.Write(event.Data[0:16]); err != nil {
		return nil
	}

	if signature.String() != "Spec ID Event03\x00" {
		return nil
	}

	algSizesStream := bytes.NewReader(event.Data[24:])

	var numAlgorithms uint32
	if err := binary.Read(algSizesStream, nativeEndian, &numAlgorithms); err != nil {
		return nil
	}

	algorithmSizes := make([]algorithmSize, numAlgorithms)

	for i := uint32(0); i < numAlgorithms; i++ {
		var algorithmId AlgorithmId
		if err := binary.Read(algSizesStream, nativeEndian, &algorithmId); err != nil {
			return nil
		}

		var digestSize uint16
		if err := binary.Read(algSizesStream, nativeEndian, &digestSize); err != nil {
			return nil
		}

		algorithmSizes[i] = algorithmSize{algorithmId, digestSize}
	}

	return algorithmSizes
}

func newLogFromReader(r io.ReadSeeker) (*Log, error) {
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	var stream stream = &stream_1_2{r}
	event, _, err := stream.ReadNextEvent()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	_, err = r.Seek(start, io.SeekStart)
	if err != nil {
		return nil, err
	}

	var format Format
	var algorithms []AlgorithmId
	if algSizes := parseTCG2LogHeader(event); algSizes != nil {
		format = Format2
		algorithms = make([]AlgorithmId, 0, len(algSizes))
		for _, algSize := range algSizes {
			knownSize, known := knownAlgorithms[algSize.algorithmId]
			if known {
				if knownSize != algSize.digestSize {
					err := &InvalidLogError{
						fmt.Sprintf("Digest size in log header for algorithm '%04x' " +
							"doesn't match expected size (size: %d, expected %d)",
							algSize.algorithmId, algSize.digestSize, knownSize)}
					return nil, err
				}
				algorithms = append(algorithms, algSize.algorithmId)
			}
		}
		stream = &stream_2{r, algSizes, false}
	} else {
		format = Format1_2
		algorithms = []AlgorithmId{AlgorithmSha1}
	}

	return &Log{format, algorithms, stream}, nil
}

func (e *InvalidLogError) Error() string {
	return fmt.Sprintf("Error whilst parsing event log: %s", e.s)
}

func (e EventType) Label() string {
	switch e {
	case EventTypePrebootCert:
		return "EV_PREBOOT_CERT"
	case EventTypePostCode:
		return "EV_POST_CODE"
	case EventTypeNoAction:
		return "EV_NO_ACTION"
	case EventTypeSeparator:
		return "EV_SEPARATOR"
	case EventTypeAction:
		return "EV_ACTION"
	case EventTypeEventTag:
		return "EV_EVENT_TAG"
	case EventTypeSCRTMContents:
		return "EV_S_CRTM_CONTENTS"
	case EventTypeSCRTMVersion:
		return "EV_S_CRTM_VERSION"
	case EventTypeCPUMicrocode:
		return "EV_CPU_MICROCODE"
	case EventTypePlatformConfigFlags:
		return "EV_PLATFORM_CONFIG_FLAGS"
	case EventTypeTableOfDevices:
		return "EV_TABLE_OF_DEVICES"
	case EventTypeCompactHash:
		return "EV_COMPACT_HASH"
	case EventTypeIPL:
		return "EV_IPL"
	case EventTypeIPLPartitionData:
		return "EV_IPL_PARTITION_DATA"
	case EventTypeNonhostCode:
		return "EV_NONHOST_CODE"
	case EventTypeNonhostConfig:
		return "EV_NONHOST_CONFIG"
	case EventTypeNonhostInfo:
		return "EV_NONHOST_INFO"
	case EventTypeOmitBootDeviceEvents:
		return "EV_OMIT_BOOT_DEVICE_EVENTS"
	case EventTypeEFIVariableDriverConfig:
		return "EV_EFI_VARIABLE_DRIVER_CONFIG"
	case EventTypeEFIVariableBoot:
		return "EV_EFI_VARIABLE_BOOT"
	case EventTypeEFIBootServicesApplication:
		return "EV_EFI_BOOT_SERVICES_APPLICATION"
	case EventTypeEFIBootServicesDriver:
		return "EV_EFI_BOOT_SERVICES_DRIVER"
	case EventTypeEFIRuntimeServicesDriver:
		return "EV_EFI_RUNTIME_SERVICES_DRIVER"
	case EventTypeEFIGPTEvent:
		return "EF_EFI_GPT_EVENT"
	case EventTypeEFIAction:
		return "EV_EFI_ACTION"
	case EventTypeEFIPlatformFirmwareBlob:
		return "EV_EFI_PLATFORM_FIRMWARE_BLOB"
	case EventTypeEFIHandoffTables:
		return "EV_EFI_HANDOFF_TABLES"
	case EventTypeEFIHCRTMEvent:
		return "EV_EFI_HCRTM_EVENT"
	case EventTypeEFIVariableAuthority:
		return "EV_EFI_VARIABLE_AUTHORITY"
	default:
		var label strings.Builder
		fmt.Fprintf(&label, "%08x", uint32(e))
		return label.String()
	}
}

func (e EventType) Format(s fmt.State, f rune) {
	switch f {
	case 's':
		fmt.Fprintf(s, "%s", e.Label())
	// case 'x':
	//     TODO
	// case 'X':
	//     TODO
	default:
		fmt.Fprintf(s, "%%!%c(tcglog.EventType=%08x)", f, uint32(e))
	}
}

func (d Digest) Format(s fmt.State, f rune) {
	switch f {
	case 's':
		fmt.Fprintf(s, "%s", hex.EncodeToString([]byte(d)))
	default:
		fmt.Fprintf(s, "%%!%c(tcglog.Digest=%s)", f, hex.EncodeToString([]byte(d)))
	}
}

func (l *Log) HasAlgorithm(alg AlgorithmId) bool {
	for _, a := range l.Algorithms {
		if a == alg {
			return true
		}
	}

	return false
}

func (l *Log) NextEvent() (*Event, error) {
	event, partial, err := l.stream.ReadNextEvent()
	if partial && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return event, err
}

func NewLogFromByteReader(reader *bytes.Reader) (*Log, error) {
	return newLogFromReader(reader)
}

func NewLogFromFile(file *os.File) (*Log, error) {
	return newLogFromReader(file)
}

func DigestLength(alg AlgorithmId) uint16 {
	return knownAlgorithms[alg]
}
