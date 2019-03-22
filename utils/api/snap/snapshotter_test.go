package snap

import (
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fearblackcat/swiftRaft/raft/raftpb"
	"github.com/fearblackcat/swiftRaft/utils/logtool"
)

var testSnap = &raftpb.Snapshot{
	Data: []byte("some snapshot"),
	Metadata: raftpb.SnapshotMetadata{
		ConfState: raftpb.ConfState{
			Nodes: []uint64{1, 2, 3},
		},
		Index: 1,
		Term:  1,
	},
}

func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ss := New(logtool.RLog, dir)
	err = ss.save(testSnap)
	if err != nil {
		t.Fatal(err)
	}

	g, err := ss.Load()
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if !reflect.DeepEqual(g, testSnap) {
		t.Errorf("snap = %#v, want %#v", g, testSnap)
	}
}

func TestBadCRC(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ss := New(logtool.RLog, dir)
	err = ss.save(testSnap)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { crcTable = crc32.MakeTable(crc32.Castagnoli) }()
	// switch to use another crc table
	// fake a crc mismatch
	crcTable = crc32.MakeTable(crc32.Koopman)

	_, err = Read(logtool.RLog, filepath.Join(dir, fmt.Sprintf("%016x-%016x.snap", 1, 1)))
	if err == nil || err != ErrCRCMismatch {
		t.Errorf("err = %v, want %v", err, ErrCRCMismatch)
	}
}

func TestFailback(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	large := fmt.Sprintf("%016x-%016x-%016x.snap", 0xFFFF, 0xFFFF, 0xFFFF)
	err = ioutil.WriteFile(filepath.Join(dir, large), []byte("bad data"), 0666)
	if err != nil {
		t.Fatal(err)
	}

	ss := New(logtool.RLog, dir)
	err = ss.save(testSnap)
	if err != nil {
		t.Fatal(err)
	}

	g, err := ss.Load()
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if !reflect.DeepEqual(g, testSnap) {
		t.Errorf("snap = %#v, want %#v", g, testSnap)
	}
	if f, err := os.Open(filepath.Join(dir, large) + ".broken"); err != nil {
		t.Fatal("broken snapshot does not exist")
	} else {
		f.Close()
	}
}

func TestSnapNames(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for i := 1; i <= 5; i++ {
		var f *os.File
		if f, err = os.Create(filepath.Join(dir, fmt.Sprintf("%d.snap", i))); err != nil {
			t.Fatal(err)
		} else {
			f.Close()
		}
	}
	ss := New(logtool.RLog, dir)
	names, err := ss.snapNames()
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if len(names) != 5 {
		t.Errorf("len = %d, want 10", len(names))
	}
	w := []string{"5.snap", "4.snap", "3.snap", "2.snap", "1.snap"}
	if !reflect.DeepEqual(names, w) {
		t.Errorf("names = %v, want %v", names, w)
	}
}

func TestLoadNewestSnap(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ss := New(logtool.RLog, dir)
	err = ss.save(testSnap)
	if err != nil {
		t.Fatal(err)
	}

	newSnap := *testSnap
	newSnap.Metadata.Index = 5
	err = ss.save(&newSnap)
	if err != nil {
		t.Fatal(err)
	}

	g, err := ss.Load()
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if !reflect.DeepEqual(g, &newSnap) {
		t.Errorf("snap = %#v, want %#v", g, &newSnap)
	}
}

func TestNoSnapshot(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ss := New(logtool.RLog, dir)
	_, err = ss.Load()
	if err != ErrNoSnapshot {
		t.Errorf("err = %v, want %v", err, ErrNoSnapshot)
	}
}

func TestEmptySnapshot(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	err = ioutil.WriteFile(filepath.Join(dir, "1.snap"), []byte(""), 0x700)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Read(logtool.RLog, filepath.Join(dir, "1.snap"))
	if err != ErrEmptySnapshot {
		t.Errorf("err = %v, want %v", err, ErrEmptySnapshot)
	}
}

// TestAllSnapshotBroken ensures snapshotter returns
// ErrNoSnapshot if all the snapshots are broken.
func TestAllSnapshotBroken(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "snapshot")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	err = ioutil.WriteFile(filepath.Join(dir, "1.snap"), []byte("bad"), 0x700)
	if err != nil {
		t.Fatal(err)
	}

	ss := New(logtool.RLog, dir)
	_, err = ss.Load()
	if err != ErrNoSnapshot {
		t.Errorf("err = %v, want %v", err, ErrNoSnapshot)
	}
}
