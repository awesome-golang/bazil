package blobs_test

import (
	"bytes"
	"io"
	"testing"

	"bazil.org/bazil/cas"
	"bazil.org/bazil/cas/blobs"
	"bazil.org/bazil/cas/chunks"
	"bazil.org/bazil/cas/chunks/mock"
)

func emptyBlob(t testing.TB, chunkStore chunks.Store) *blobs.Blob {
	blob, err := blobs.Open(
		chunkStore,
		blobs.EmptyManifest("footype"),
	)
	if err != nil {
		t.Fatalf("cannot open blob: %v", err)
	}
	return blob
}

func TestOpenNoType(t *testing.T) {
	_, err := blobs.Open(mock.NeverUsed{}, &blobs.Manifest{
		// no Type
		ChunkSize: blobs.MinChunkSize,
		Fanout:    2,
	})
	if g, e := err, blobs.MissingType; g != e {
		t.Fatalf("bad error: %v != %v", g, e)
	}
}

func TestEmptyRead(t *testing.T) {
	blob := emptyBlob(t, mock.NeverUsed{})
	buf := make([]byte, 10)
	n, err := blob.ReadAt(buf, 3)
	if g, e := err, io.EOF; g != e {
		t.Errorf("expected EOF: %v != %v", g, e)
	}
	if g, e := n, 0; g != e {
		t.Errorf("expected to read 0 bytes: %v != %v", g, e)
	}
}

func TestSparseRead(t *testing.T) {
	const chunkSize = 4096
	blob, err := blobs.Open(
		&mock.InMemory{},
		&blobs.Manifest{
			Type:      "footype",
			Size:      100,
			ChunkSize: chunkSize,
			Fanout:    2,
		},
	)
	buf := make([]byte, 10)
	n, err := blob.ReadAt(buf, 3)
	if err != nil {
		t.Errorf("unexpected read error: %v", err)
	}
	if g, e := n, 10; g != e {
		t.Errorf("expected to read 0 bytes: %v != %v", g, e)
	}
}

func TestEmptySave(t *testing.T) {
	blob := emptyBlob(t, mock.NeverUsed{})
	saved, err := blob.Save()
	if err != nil {
		t.Errorf("unexpected error from Save: %v", err)
	}
	if g, e := saved.Type, "footype"; g != e {
		t.Errorf("unexpected type: %v != %v", g, e)
	}
	if g, e := saved.Root, cas.Empty; g != e {
		t.Errorf("unexpected key: %v != %v", g, e)
	}
	if g, e := saved.Size, uint64(0); g != e {
		t.Errorf("unexpected size: %v != %v", g, e)
	}
}

func TestEmptyDirtySave(t *testing.T) {
	blob := emptyBlob(t, &mock.InMemory{})
	n, err := blob.WriteAt([]byte{0x00}, 0)
	if err != nil {
		t.Errorf("unexpected error from WriteAt: %v", err)
	}
	if g, e := n, 1; g != e {
		t.Errorf("unexpected write length: %v != %v", g, e)
	}
	if g, e := blob.Size(), uint64(1); g != e {
		t.Errorf("unexpected manifest size: %v != %v", g, e)
	}

	saved, err := blob.Save()
	if err != nil {
		t.Errorf("unexpected error from Save: %v", err)
	}
	if g, e := saved.Root, cas.Empty; g != e {
		t.Errorf("unexpected key: %v != %v", g, e)
	}
	if g, e := saved.Size, uint64(1); g != e {
		t.Errorf("unexpected size: %v != %v", g, e)
	}
}

var GREETING = []byte("hello, world\n")

func TestWriteAndRead(t *testing.T) {
	blob := emptyBlob(t, &mock.InMemory{})
	n, err := blob.WriteAt(GREETING, 0)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if g, e := n, len(GREETING); g != e {
		t.Errorf("unexpected write length: %v != %v", g, e)
	}
	if g, e := blob.Size(), uint64(len(GREETING)); g != e {
		t.Errorf("unexpected manifest size: %v != %v", g, e)
	}

	// do +1 to trigger us seeing EOF too
	buf := make([]byte, len(GREETING)+1)
	n, err = blob.ReadAt(buf, 0)
	if err != io.EOF {
		t.Errorf("expected read EOF: %v", err)
	}
	if g, e := n, len(GREETING); g != e {
		t.Errorf("unexpected read length: %v != %v", g, e)
	}
	buf = buf[:n]
	if !bytes.Equal(GREETING, buf) {
		t.Errorf("unexpected read data: %q", buf)
	}
}

func TestWriteSaveAndRead(t *testing.T) {
	chunkStore := &mock.InMemory{}
	var saved *blobs.Manifest
	{
		blob := emptyBlob(t, chunkStore)
		n, err := blob.WriteAt(GREETING, 0)
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if g, e := n, len(GREETING); g != e {
			t.Errorf("unexpected write length: %v != %v", g, e)
		}
		if g, e := blob.Size(), uint64(len(GREETING)); g != e {
			t.Errorf("unexpected manifest size: %v != %v", g, e)
		}
		saved, err = blob.Save()
		if err != nil {
			t.Fatalf("unexpected error from Save: %v", err)
		}
	}

	b, err := blobs.Open(chunkStore, saved)
	if err != nil {
		t.Fatalf("cannot open saved blob: %v", err)
	}
	// do +1 to trigger us seeing EOF too
	buf := make([]byte, len(GREETING)+1)
	n, err := b.ReadAt(buf, 0)
	if err != io.EOF {
		t.Errorf("expected read EOF: %v", err)
	}
	if g, e := n, len(GREETING); g != e {
		t.Errorf("unexpected read length: %v != %v", g, e)
	}
	buf = buf[:n]
	if !bytes.Equal(GREETING, buf) {
		t.Errorf("unexpected read data: %q", buf)
	}
}

func TestWriteSaveLoopAndRead(t *testing.T) {
	const chunkSize = 4096
	const fanout = 2
	chunkStore := &mock.InMemory{}
	blob, err := blobs.Open(chunkStore, &blobs.Manifest{
		Type:      "footype",
		ChunkSize: chunkSize,
		Fanout:    fanout,
	})
	if err != nil {
		t.Fatalf("cannot open blob: %v", err)
	}
	// not exactly sure where this magic number comes from :(
	greeting := bytes.Repeat(GREETING, 40330)

	var prev *cas.Key
	for i := 0; i <= 2; i++ {
		n, err := blob.WriteAt(greeting, 0)
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if g, e := n, len(greeting); g != e {
			t.Errorf("unexpected write length: %v != %v", g, e)
		}
		if g, e := blob.Size(), uint64(len(greeting)); g != e {
			t.Errorf("unexpected manifest size: %v != %v", g, e)
		}
		saved, err := blob.Save()
		if err != nil {
			t.Fatalf("unexpected error from Save: %v", err)
		}
		t.Logf("saved %v size=%d", saved.Root, saved.Size)
		if prev != nil {
			if g, e := saved.Root, *prev; g != e {
				t.Errorf("unexpected key: %q != %q", g, e)
			}
		}
		tmp := saved.Root
		prev = &tmp
	}

	// do +1 to trigger us seeing EOF too
	buf := make([]byte, len(greeting)+1)
	n, err := blob.ReadAt(buf, 0)
	if err != io.EOF {
		t.Errorf("expected read EOF: %v", err)
	}
	if g, e := n, len(greeting); g != e {
		t.Errorf("unexpected read length: %v != %v", g, e)
	}
	buf = buf[:n]
	if !bytes.Equal(greeting, buf) {
		// assumes len > 100, which we know is true
		t.Errorf("unexpected read data %q..%q", buf[:100], buf[len(buf)-100:])
	}
}

func TestWriteSaveAndReadLarge(t *testing.T) {
	const chunkSize = 4096
	const fanout = 2
	chunkStore := &mock.InMemory{}
	// just enough to span multiple chunks
	greeting := bytes.Repeat(GREETING, chunkSize/len(GREETING)+1)

	var saved *blobs.Manifest
	{
		blob, err := blobs.Open(chunkStore, &blobs.Manifest{
			Type:      "footype",
			ChunkSize: chunkSize,
			Fanout:    fanout,
		})
		if err != nil {
			t.Fatalf("cannot open blob: %v", err)
		}
		n, err := blob.WriteAt(greeting, 0)
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if g, e := n, len(greeting); g != e {
			t.Errorf("unexpected write length: %v != %v", g, e)
		}
		if g, e := blob.Size(), uint64(len(greeting)); g != e {
			t.Errorf("unexpected manifest size: %v != %v", g, e)
		}
		saved, err = blob.Save()
		if err != nil {
			t.Fatalf("unexpected error from Save: %v", err)
		}
	}

	t.Logf("saved manifest: %+v", saved)
	b, err := blobs.Open(chunkStore, saved)
	if err != nil {
		t.Fatalf("cannot open saved blob: %v", err)
	}
	// do +1 to trigger us seeing EOF too
	buf := make([]byte, len(greeting)+1)
	n, err := b.ReadAt(buf, 0)
	if err != io.EOF {
		t.Errorf("expected read EOF: %v", err)
	}
	if g, e := n, len(greeting); g != e {
		t.Errorf("unexpected read length: %v != %v", g, e)
	}
	buf = buf[:n]
	if !bytes.Equal(greeting, buf) {
		t.Errorf("unexpected read data: %q", buf)
	}
}

func TestWriteSparse(t *testing.T) {
	const chunkSize = 4096
	chunkStore := &mock.InMemory{}
	blob, err := blobs.Open(chunkStore, &blobs.Manifest{
		Type:      "footype",
		ChunkSize: chunkSize,
		Fanout:    2,
	})
	if err != nil {
		t.Fatalf("cannot open blob: %v", err)
	}

	// note: gap after end of first chunk
	n, err := blob.WriteAt([]byte{'x'}, chunkSize+3)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if g, e := n, 1; g != e {
		t.Errorf("unexpected write length: %v != %v", g, e)
	}
	if g, e := blob.Size(), uint64(chunkSize)+3+1; g != e {
		t.Errorf("unexpected manifest size: %v != %v", g, e)
	}

	// read exactly a chunksize to access only the hole
	buf := make([]byte, 1)
	n, err = blob.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if g, e := n, len(buf); g != e {
		t.Errorf("unexpected read length: %v != %v", g, e)
	}
	buf = buf[:n]
	if !bytes.Equal([]byte{0}, buf) {
		t.Errorf("unexpected read data: %q", buf)
	}
}

func TestWriteSparseBoundary(t *testing.T) {
	const chunkSize = 4096
	chunkStore := &mock.InMemory{}
	blob, err := blobs.Open(chunkStore, &blobs.Manifest{
		Type:      "footype",
		ChunkSize: chunkSize,
		Fanout:    2,
	})
	if err != nil {
		t.Fatalf("cannot open blob: %v", err)
	}

	n, err := blob.WriteAt([]byte{'x', 'y'}, chunkSize)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if g, e := n, 2; g != e {
		t.Errorf("unexpected write length: %v != %v", g, e)
	}
	if g, e := blob.Size(), uint64(chunkSize)+2; g != e {
		t.Errorf("unexpected manifest size: %v != %v", g, e)
	}

	// access only the hole
	buf := make([]byte, 1)
	n, err = blob.ReadAt(buf, chunkSize)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if g, e := n, len(buf); g != e {
		t.Errorf("unexpected read length: %v != %v", g, e)
	}
	buf = buf[:n]
	if !bytes.Equal([]byte{'x'}, buf) {
		t.Errorf("unexpected read data: %q", buf)
	}
}

func TestWriteAndSave(t *testing.T) {
	chunkStore := &mock.InMemory{}
	blob := emptyBlob(t, chunkStore)

	n, err := blob.WriteAt(GREETING, 0)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if g, e := n, len(GREETING); g != e {
		t.Errorf("unexpected write length: %v != %v", g, e)
	}

	saved, err := blob.Save()
	if err != nil {
		t.Fatalf("unexpected error from Save: %v", err)
	}
	if g, e := saved.Root.String(), "cb53f96a3c9d1e087649fd8a3415994eb635d0bb9ba9b8cebceea313366fd34a19b41b665237d212f91ec60dc21a485c777c3d89ffd1caae31daf09a18562560"; g != e {
		t.Errorf("unexpected key: %q != %q", g, e)
	}
	if g, e := saved.Size, uint64(len(GREETING)); g != e {
		t.Errorf("unexpected size: %v != %v", g, e)
	}
}

func TestWriteAndSaveLarge(t *testing.T) {
	const chunkSize = 4096
	const fanout = 64
	chunkStore := &mock.InMemory{}
	blob, err := blobs.Open(chunkStore, &blobs.Manifest{
		Type:      "footype",
		ChunkSize: chunkSize,
		Fanout:    fanout,
	})
	if err != nil {
		t.Fatalf("cannot open blob: %v", err)
	}
	n, err := blob.WriteAt(bytes.Join([][]byte{
		bytes.Repeat([]byte{'x'}, chunkSize),
		bytes.Repeat([]byte{'y'}, chunkSize),
	}, []byte{}), 0)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if g, e := n, 2*chunkSize; g != e {
		t.Errorf("unexpected write length: %v != %v", g, e)
	}

	saved, err := blob.Save()
	if err != nil {
		t.Fatalf("unexpected error from Save: %v", err)
	}
	if g, e := saved.Root.String(), "9f3f6815c7680f98e00fe9ab5edc85ba3f4ceb657b9562c35b5a865d970ea3270bab8c7aa3162cbaaa966ad84330f34a22aa9539b4c416f858c35c0775482665"; g != e {
		t.Errorf("unexpected key: %q != %q", g, e)
	}
	if g, e := saved.Size, uint64(chunkSize+chunkSize); g != e {
		t.Errorf("unexpected size: %v != %v", g, e)
	}
}
