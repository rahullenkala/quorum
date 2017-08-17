package procfs

// While implementing parsing of /proc/[pid]/mountstats, this blog was used
// heavily as a reference:
//   https://utcc.utoronto.ca/~cks/space/blog/linux/NFSMountstatsIndex
//
// Special thanks to Chris Siebenmann for all of his posts explaining the
// various statistics available for NFS.

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Constants shared between multiple functions.
const (
	deviceEntryLen = 8

	fieldBytesLen  = 8
	fieldEventsLen = 27

	statVersion10 = "1.0"
	statVersion11 = "1.1"

	fieldTransport10Len = 10
	fieldTransport11Len = 13
)

// A Mount is a device mount parsed from /proc/[pid]/mountstats.
type Mount struct {
	// Name of the device.
	Device string
	// The mount point of the device.
	Mount string
	// The filesystem type used by the device.
	Type string
	// If available additional statistics related to this Mount.
	// Use a type assertion to determine if additional statistics are available.
	Stats MountStats
}

// A MountStats is a type which contains detailed statistics for a specific
// type of Mount.
type MountStats interface {
	mountStats()
}

// A MountStatsNFS is a MountStats implementation for NFSv3 and v4 mounts.
type MountStatsNFS struct {
	// The version of statistics provided.
	StatVersion string
	// The age of the NFS mount.
	Age time.Duration
	// Statistics related to byte counters for various operations.
	Bytes NFSBytesStats
	// Statistics related to various NFS event occurrences.
	Events NFSEventsStats
	// Statistics broken down by filesystem operation.
	Operations []NFSOperationStats
	// Statistics about the NFS RPC transport.
	Transport NFSTransportStats
}

// mountStats implements MountStats.
func (m MountStatsNFS) mountStats() {}

// A NFSBytesStats contains statistics about the number of bytes read and written
// by an NFS client to and from an NFS server.
type NFSBytesStats struct {
	// Number of bytes read using the read() syscall.
	Read int
	// Number of bytes written using the write() syscall.
	Write int
	// Number of bytes read using the read() syscall in O_DIRECT mode.
	DirectRead int
	// Number of bytes written using the write() syscall in O_DIRECT mode.
	DirectWrite int
	// Number of bytes read from the NFS server, in total.
	ReadTotal int
	// Number of bytes written to the NFS server, in total.
	WriteTotal int
	// Number of pages read directly via mmap()'d files.
	ReadPages int
	// Number of pages written directly via mmap()'d files.
	WritePages int
}

// A NFSEventsStats contains statistics about NFS event occurrences.
type NFSEventsStats struct {
	// Number of times cached inode attributes are re-validated from the server.
	InodeRevalidate int
	// Number of times cached dentry nodes are re-validated from the server.
	DnodeRevalidate int
	// Number of times an inode cache is cleared.
	DataInvalidate int
	// Number of times cached inode attributes are invalidated.
	AttributeInvalidate int
	// Number of times files or directories have been open()'d.
	VFSOpen int
	// Number of times a directory lookup has occurred.
	VFSLookup int
	// Number of times permissions have been checked.
	VFSAccess int
	// Number of updates (and potential writes) to pages.
	VFSUpdatePage int
	// Number of pages read directly via mmap()'d files.
	VFSReadPage int
	// Number of times a group of pages have been read.
	VFSReadPages int
	// Number of pages written directly via mmap()'d files.
	VFSWritePage int
	// Number of times a group of pages have been written.
	VFSWritePages int
	// Number of times directory entries have been read with getdents().
	VFSGetdents int
	// Number of times attributes have been set on inodes.
	VFSSetattr int
	// Number of pending writes that have been forcefully flushed to the server.
	VFSFlush int
	// Number of times fsync() has been called on directories and files.
	VFSFsync int
	// Number of times locking has been attemped on a file.
	VFSLock int
	// Number of times files have been closed and released.
	VFSFileRelease int
	// Unknown.  Possibly unused.
	CongestionWait int
	// Number of times files have been truncated.
	Truncation int
	// Number of times a file has been grown due to writes beyond its existing end.
	WriteExtension int
	// Number of times a file was removed while still open by another process.
	SillyRename int
	// Number of times the NFS server gave less data than expected while reading.
	ShortRead int
	// Number of times the NFS server wrote less data than expected while writing.
	ShortWrite int
	// Number of times the NFS server indicated EJUKEBOX; retrieving data from
	// offline storage.
	JukeboxDelay int
	// Number of NFS v4.1+ pNFS reads.
	PNFSRead int
	// Number of NFS v4.1+ pNFS writes.
	PNFSWrite int
}

// A NFSOperationStats contains statistics for a single operation.
type NFSOperationStats struct {
	// The name of the operation.
	Operation string
	// Number of requests performed for this operation.
	Requests int
	// Number of times an actual RPC request has been transmitted for this operation.
	Transmissions int
	// Number of times a request has had a major timeout.
	MajorTimeouts int
	// Number of bytes sent for this operation, including RPC headers and payload.
	BytesSent int
	// Number of bytes received for this operation, including RPC headers and payload.
	BytesReceived int
	// Duration all requests spent queued for transmission before they were sent.
	CumulativeQueueTime time.Duration
	// Duration it took to get a reply back after the request was transmitted.
	CumulativeTotalResponseTime time.Duration
	// Duration from when a request was enqueued to when it was completely handled.
	CumulativeTotalRequestTime time.Duration
}

// A NFSTransportStats contains statistics for the NFS mount RPC requests and
// responses.
type NFSTransportStats struct {
	// The local port used for the NFS mount.
	Port int
	// Number of times the client has had to establish a connection from scratch
	// to the NFS server.
	Bind int
	// Number of times the client has made a TCP connection to the NFS server.
	Connect int
	// Duration (in jiffies, a kernel internal unit of time) the NFS mount has
	// spent waiting for connections to the server to be established.
	ConnectIdleTime int
	// Duration since the NFS mount last saw any RPC traffic.
	IdleTime time.Duration
	// Number of RPC requests for this mount sent to the NFS server.
	Sends int
	// Number of RPC responses for this mount received from the NFS server.
	Receives int
	// Number of times the NFS server sent a response with a transaction ID
	// unknown to this client.
	BadTransactionIDs int
	// A running counter, incremented on each request as the current difference
	// ebetween sends and receives.
	CumulativeActiveRequests int
	// A running counter, incremented on each request by the current backlog
	// queue size.
	CumulativeBacklog int

	// Stats below only available with stat version 1.1.

	// Maximum number of simultaneously active RPC requests ever used.
	MaximumRPCSlotsUsed int
	// A running counter, incremented on each request as the current size of the
	// sending queue.
	CumulativeSendingQueue int
	// A running counter, incremented on each request as the current size of the
	// pending queue.
	CumulativePendingQueue int
}

// parseMountStats parses a /proc/[pid]/mountstats file and returns a slice
// of Mount structures containing detailed information about each mount.
// If available, statistics for each mount are parsed as well.
func parseMountStats(r io.Reader) ([]*Mount, error) {
	const (
		device            = "device"
		statVersionPrefix = "statvers="

		nfs3Type = "nfs"
		nfs4Type = "nfs4"
	)

	var mounts []*Mount

	s := bufio.NewScanner(r)
	for s.Scan() {
		// Only look for device entries in this function
		ss := strings.Fields(string(s.Bytes()))
		if len(ss) == 0 || ss[0] != device {
			continue
		}

		m, err := parseMount(ss)
		if err != nil {
			return nil, err
		}

		// Does this mount also possess statistics information?
		if len(ss) > deviceEntryLen {
			// Only NFSv3 and v4 are supported for parsing statistics
			if m.Type != nfs3Type && m.Type != nfs4Type {
				return nil, fmt.Errorf("cannot parse MountStats for fstype %q", m.Type)
			}

			statVersion := strings.TrimPrefix(ss[8], statVersionPrefix)

			stats, err := parseMountStatsNFS(s, statVersion)
			if err != nil {
				return nil, err
			}

			m.Stats = stats
		}

		mounts = append(mounts, m)
	}

	return mounts, s.Err()
}

// parseMount parses an entry in /proc/[pid]/mountstats in the format:
//   device [device] mounted on [mount] with fstype [type]
func parseMount(ss []string) (*Mount, error) {
	if len(ss) < deviceEntryLen {
		return nil, fmt.Errorf("invalid device entry: %v", ss)
	}

	// Check for specific words appearing at specific indices to ensure
	// the format is consistent with what we expect
	format := []struct {
		i int
		s string
	}{
		{i: 0, s: "device"},
		{i: 2, s: "mounted"},
		{i: 3, s: "on"},
		{i: 5, s: "with"},
		{i: 6, s: "fstype"},
	}

	for _, f := range format {
		if ss[f.i] != f.s {
			return nil, fmt.Errorf("invalid device entry: %v", ss)
		}
	}

	return &Mount{
		Device: ss[1],
		Mount:  ss[4],
		Type:   ss[7],
	}, nil
}

// parseMountStatsNFS parses a MountStatsNFS by scanning additional information
// related to NFS statistics.
func parseMountStatsNFS(s *bufio.Scanner, statVersion string) (*MountStatsNFS, error) {
	// Field indicators for parsing specific types of data
	const (
		fieldAge        = "age:"
		fieldBytes      = "bytes:"
		fieldEvents     = "events:"
		fieldPerOpStats = "per-op"
		fieldTransport  = "xprt:"
	)

	stats := &MountStatsNFS{
		StatVersion: statVersion,
	}

	for s.Scan() {
		ss := strings.Fields(string(s.Bytes()))
		if len(ss) == 0 {
			break
		}
		if len(ss) < 2 {
			return nil, fmt.Errorf("not enough information for NFS stats: %v", ss)
		}

		switch ss[0] {
		case fieldAge:
			// Age integer is in seconds
			d, err := time.ParseDuration(ss[1] + "s")
			if err != nil {
				return nil, err
			}

			stats.Age = d
		case fieldBytes:
			bstats, err := parseNFSBytesStats(ss[1:])
			if err != nil {
				return nil, err
			}

			stats.Bytes = *bstats
		case fieldEvents:
			estats, err := parseNFSEventsStats(ss[1:])
			if err != nil {
				return nil, err
			}

			stats.Events = *estats
		case fieldTransport:
			if len(ss) < 3 {
				return nil, fmt.Errorf("not enough information for NFS transport stats: %v", ss)
			}

			tstats, err := parseNFSTransportStats(ss[2:], statVersion)
			if err != nil {
				return nil, err
			}

			stats.Transport = *tstats
		}

		// When encountering "per-operation statistics", we must break this
		// loop and parse them seperately to ensure we can terminate parsing
		// before reaching another device entry; hence why this 'if' statement
		// is not just another switch case
		if ss[0] == fieldPerOpStats {
			break
		}
	}

	if err := s.Err(); err != nil {
		return nil, err
	}

	// NFS per-operation stats appear last before the next device entry
	perOpStats, err := parseNFSOperationStats(s)
	if err != nil {
		return nil, err
	}

	stats.Operations = perOpStats

	return stats, nil
}

// parseNFSBytesStats parses a NFSBytesStats line using an input set of
// integer fields.
func parseNFSBytesStats(ss []string) (*NFSBytesStats, error) {
	if len(ss) != fieldBytesLen {
		return nil, fmt.Errorf("invalid NFS bytes stats: %v", ss)
	}

	ns := make([]int, 0, fieldBytesLen)
	for _, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}

		ns = append(ns, n)
	}

	return &NFSBytesStats{
		Read:        ns[0],
		Write:       ns[1],
		DirectRead:  ns[2],
		DirectWrite: ns[3],
		ReadTotal:   ns[4],
		WriteTotal:  ns[5],
		ReadPages:   ns[6],
		WritePages:  ns[7],
	}, nil
}

// parseNFSEventsStats parses a NFSEventsStats line using an input set of
// integer fields.
func parseNFSEventsStats(ss []string) (*NFSEventsStats, error) {
	if len(ss) != fieldEventsLen {
		return nil, fmt.Errorf("invalid NFS events stats: %v", ss)
	}

	ns := make([]int, 0, fieldEventsLen)
	for _, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}

		ns = append(ns, n)
	}

	return &NFSEventsStats{
		InodeRevalidate:     ns[0],
		DnodeRevalidate:     ns[1],
		DataInvalidate:      ns[2],
		AttributeInvalidate: ns[3],
		VFSOpen:             ns[4],
		VFSLookup:           ns[5],
		VFSAccess:           ns[6],
		VFSUpdatePage:       ns[7],
		VFSReadPage:         ns[8],
		VFSReadPages:        ns[9],
		VFSWritePage:        ns[10],
		VFSWritePages:       ns[11],
		VFSGetdents:         ns[12],
		VFSSetattr:          ns[13],
		VFSFlush:            ns[14],
		VFSFsync:            ns[15],
		VFSLock:             ns[16],
		VFSFileRelease:      ns[17],
		CongestionWait:      ns[18],
		Truncation:          ns[19],
		WriteExtension:      ns[20],
		SillyRename:         ns[21],
		ShortRead:           ns[22],
		ShortWrite:          ns[23],
		JukeboxDelay:        ns[24],
		PNFSRead:            ns[25],
		PNFSWrite:           ns[26],
	}, nil
}

// parseNFSOperationStats parses a slice of NFSOperationStats by scanning
// additional information about per-operation statistics until an empty
// line is reached.
func parseNFSOperationStats(s *bufio.Scanner) ([]NFSOperationStats, error) {
	const (
		// Number of expected fields in each per-operation statistics set
		numFields = 9
	)

	var ops []NFSOperationStats

	for s.Scan() {
		ss := strings.Fields(string(s.Bytes()))
		if len(ss) == 0 {
			// Must break when reading a blank line after per-operation stats to
			// enable top-level function to parse the next device entry
			break
		}

		if len(ss) != numFields {
			return nil, fmt.Errorf("invalid NFS per-operations stats: %v", ss)
		}

		// Skip string operation name for integers
		ns := make([]int, 0, numFields-1)
		for _, st := range ss[1:] {
			n, err := strconv.Atoi(st)
			if err != nil {
				return nil, err
			}

			ns = append(ns, n)
		}

		ops = append(ops, NFSOperationStats{
			Operation:                   strings.TrimSuffix(ss[0], ":"),
			Requests:                    ns[0],
			Transmissions:               ns[1],
			MajorTimeouts:               ns[2],
			BytesSent:                   ns[3],
			BytesReceived:               ns[4],
			CumulativeQueueTime:         time.Duration(ns[5]) * time.Millisecond,
			CumulativeTotalResponseTime: time.Duration(ns[6]) * time.Millisecond,
			CumulativeTotalRequestTime:  time.Duration(ns[7]) * time.Millisecond,
		})
	}

	return ops, s.Err()
}

// parseNFSTransportStats parses a NFSTransportStats line using an input set of
// integer fields matched to a specific stats version.
func parseNFSTransportStats(ss []string, statVersion string) (*NFSTransportStats, error) {
	switch statVersion {
	case statVersion10:
		if len(ss) != fieldTransport10Len {
			return nil, fmt.Errorf("invalid NFS transport stats 1.0 statement: %v", ss)
		}
	case statVersion11:
		if len(ss) != fieldTransport11Len {
			return nil, fmt.Errorf("invalid NFS transport stats 1.1 statement: %v", ss)
		}
	default:
		return nil, fmt.Errorf("unrecognized NFS transport stats version: %q", statVersion)
	}

	// Allocate enough for v1.1 stats since zero value for v1.1 stats will be okay
	// in a v1.0 response
	ns := make([]int, 0, fieldTransport11Len)
	for _, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}

		ns = append(ns, n)
	}

	return &NFSTransportStats{
		Port:                     ns[0],
		Bind:                     ns[1],
		Connect:                  ns[2],
		ConnectIdleTime:          ns[3],
		IdleTime:                 time.Duration(ns[4]) * time.Second,
		Sends:                    ns[5],
		Receives:                 ns[6],
		BadTransactionIDs:        ns[7],
		CumulativeActiveRequests: ns[8],
		CumulativeBacklog:        ns[9],
		MaximumRPCSlotsUsed:      ns[10],
		CumulativeSendingQueue:   ns[11],
		CumulativePendingQueue:   ns[12],
	}, nil
}