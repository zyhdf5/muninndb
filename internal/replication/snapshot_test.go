package replication

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/transport/mbp"
	"github.com/vmihailenco/msgpack/v5"
)

// newTestDB opens a temporary Pebble DB and registers cleanup.
func newTestDB(t *testing.T) *pebble.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// snapPipeConn creates a pair of PeerConns connected via net.Pipe().
// Returns (senderConn, receiverConn).
func snapPipeConn(t *testing.T) (*PeerConn, *PeerConn) {
	t.Helper()
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		c1.Close()
		c2.Close()
	})

	sender := &PeerConn{
		nodeID: "sender",
		addr:   "pipe",
		conn:   c1,
	}
	receiver := &PeerConn{
		nodeID: "receiver",
		addr:   "pipe",
		conn:   c2,
	}
	return sender, receiver
}

func TestSnapshotSender_Send_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	repLog := NewReplicationLog(db)
	sender := NewSnapshotSender(db, repLog)

	senderConn, receiverConn := snapPipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	seqCh := make(chan uint64, 1)
	go func() {
		seq, err := sender.Send(ctx, senderConn)
		seqCh <- seq
		errCh <- err
	}()

	// Receiver side: read header.
	headerType, headerPayload, err := receiverConn.Receive()
	if err != nil {
		t.Fatalf("receive header: %v", err)
	}
	if headerType != mbp.TypeSnapHeader {
		t.Fatalf("expected TypeSnapHeader, got 0x%02x", headerType)
	}

	var header mbp.SnapHeader
	if err := msgpack.Unmarshal(headerPayload, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header.SnapshotSeq != 0 {
		t.Errorf("SnapshotSeq = %d, want 0", header.SnapshotSeq)
	}
	if header.TotalKeys != 0 {
		t.Errorf("TotalKeys = %d, want 0", header.TotalKeys)
	}

	// Send ack.
	if err := receiverConn.Send(mbp.TypeSnapAck, nil); err != nil {
		t.Fatalf("send ack: %v", err)
	}

	// Read final chunk (Done=true).
	chunkType, chunkPayload, err := receiverConn.Receive()
	if err != nil {
		t.Fatalf("receive chunk: %v", err)
	}
	if chunkType != mbp.TypeSnapChunk {
		t.Fatalf("expected TypeSnapChunk, got 0x%02x", chunkType)
	}

	var chunk mbp.SnapChunk
	if err := msgpack.Unmarshal(chunkPayload, &chunk); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	if !chunk.LastChunk {
		t.Error("expected LastChunk=true")
	}
	if len(chunk.Pairs) != 0 {
		t.Errorf("expected 0 pairs, got %d", len(chunk.Pairs))
	}

	// Read complete.
	completeType, _, err := receiverConn.Receive()
	if err != nil {
		t.Fatalf("receive complete: %v", err)
	}
	if completeType != mbp.TypeSnapComplete {
		t.Fatalf("expected TypeSnapComplete, got 0x%02x", completeType)
	}

	// Check sender result.
	if err := <-errCh; err != nil {
		t.Fatalf("sender error: %v", err)
	}
	if seq := <-seqCh; seq != 0 {
		t.Errorf("returned seq = %d, want 0", seq)
	}
}

func TestSnapshotSender_Send_WithData(t *testing.T) {
	senderDB := newTestDB(t)
	receiverDB := newTestDB(t)
	repLog := NewReplicationLog(senderDB)

	// Write 1000 KV pairs directly to senderDB.
	for i := 0; i < 1000; i++ {
		key := []byte(fmt.Sprintf("key-%04d", i))
		val := []byte(fmt.Sprintf("value-%04d", i))
		if err := senderDB.Set(key, val, pebble.NoSync); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}

	// Also advance repLog to seq=5 to verify header carries the seq.
	for i := 0; i < 5; i++ {
		if _, err := repLog.Append(OpSet, []byte("rk"), []byte("rv")); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	sender := NewSnapshotSender(senderDB, repLog)
	receiver := NewSnapshotReceiver(receiverDB)

	senderConn, receiverConn := snapPipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sendErrCh := make(chan error, 1)
	sendSeqCh := make(chan uint64, 1)
	go func() {
		seq, err := sender.Send(ctx, senderConn)
		sendSeqCh <- seq
		sendErrCh <- err
	}()

	recvErrCh := make(chan error, 1)
	recvSeqCh := make(chan uint64, 1)
	go func() {
		seq, err := receiver.Receive(ctx, receiverConn)
		recvSeqCh <- seq
		recvErrCh <- err
	}()

	// Wait for both.
	if err := <-sendErrCh; err != nil {
		t.Fatalf("sender error: %v", err)
	}
	if err := <-recvErrCh; err != nil {
		t.Fatalf("receiver error: %v", err)
	}

	sendSeq := <-sendSeqCh
	recvSeq := <-recvSeqCh
	if sendSeq != recvSeq {
		t.Errorf("seq mismatch: sender=%d, receiver=%d", sendSeq, recvSeq)
	}

	// Verify receiver has all 1000 user keys.
	for i := 0; i < 1000; i++ {
		key := []byte(fmt.Sprintf("key-%04d", i))
		expectedVal := []byte(fmt.Sprintf("value-%04d", i))
		val, closer, err := receiverDB.Get(key)
		if err != nil {
			t.Errorf("key %s missing: %v", key, err)
			continue
		}
		if !bytes.Equal(val, expectedVal) {
			t.Errorf("key %s: got %q, want %q", key, val, expectedVal)
		}
		closer.Close()
	}
}

func TestSnapshotSender_ConcurrentLimit(t *testing.T) {
	db := newTestDB(t)
	repLog := NewReplicationLog(db)
	sender := NewSnapshotSender(db, repLog)

	conn1Sender, conn1Receiver := snapPipeConn(t)
	conn2Sender, _ := snapPipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start first Send — it will block waiting for ack.
	firstStarted := make(chan struct{})
	firstErr := make(chan error, 1)
	go func() {
		close(firstStarted)
		_, err := sender.Send(ctx, conn1Sender)
		firstErr <- err
	}()

	<-firstStarted
	// Read the header so we know Send() is past the mutex.
	_, _, err := conn1Receiver.Receive()
	if err != nil {
		t.Fatalf("receive header from first send: %v", err)
	}

	// Second Send should fail immediately.
	_, err = sender.Send(ctx, conn2Sender)
	if err != ErrSnapshotInProgress {
		t.Errorf("second Send err = %v, want ErrSnapshotInProgress", err)
	}

	// Unblock first: send ack then drain.
	cancel()
	<-firstErr
}

func TestSnapshotReceiver_Receive(t *testing.T) {
	senderDB := newTestDB(t)
	receiverDB := newTestDB(t)
	repLog := NewReplicationLog(senderDB)

	// Write 100 pairs.
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("k%03d", i))
		val := []byte(fmt.Sprintf("v%03d", i))
		if err := senderDB.Set(key, val, pebble.NoSync); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	sender := NewSnapshotSender(senderDB, repLog)
	receiver := NewSnapshotReceiver(receiverDB)

	senderConn, receiverConn := snapPipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	var sendErr, recvErr error
	var recvSeq uint64

	go func() {
		defer wg.Done()
		_, sendErr = sender.Send(ctx, senderConn)
	}()

	go func() {
		defer wg.Done()
		recvSeq, recvErr = receiver.Receive(ctx, receiverConn)
	}()

	wg.Wait()

	if sendErr != nil {
		t.Fatalf("sender: %v", sendErr)
	}
	if recvErr != nil {
		t.Fatalf("receiver: %v", recvErr)
	}
	if recvSeq != 0 {
		t.Errorf("recvSeq = %d, want 0", recvSeq)
	}

	// Verify all 100 keys.
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("k%03d", i))
		expected := []byte(fmt.Sprintf("v%03d", i))
		val, closer, err := receiverDB.Get(key)
		if err != nil {
			t.Errorf("missing key %s: %v", key, err)
			continue
		}
		if !bytes.Equal(val, expected) {
			t.Errorf("key %s: %q != %q", key, val, expected)
		}
		closer.Close()
	}
}

func TestSnapshot_RoundTrip_LargeData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large data test in short mode")
	}

	senderDB := newTestDB(t)
	receiverDB := newTestDB(t)
	repLog := NewReplicationLog(senderDB)

	// Write 50000 KV pairs.
	const numKeys = 50000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("key-%06d", i))
		val := []byte(fmt.Sprintf("value-%06d", i))
		if err := senderDB.Set(key, val, pebble.NoSync); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}

	sender := NewSnapshotSender(senderDB, repLog)
	// Increase rate limit to speed up test.
	sender.rateLimit = 1 << 30 // ~1 GB/s
	receiver := NewSnapshotReceiver(receiverDB)

	senderConn, receiverConn := snapPipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	var sendErr, recvErr error

	go func() {
		defer wg.Done()
		_, sendErr = sender.Send(ctx, senderConn)
	}()

	go func() {
		defer wg.Done()
		_, recvErr = receiver.Receive(ctx, receiverConn)
	}()

	wg.Wait()

	if sendErr != nil {
		t.Fatalf("sender: %v", sendErr)
	}
	if recvErr != nil {
		t.Fatalf("receiver: %v", recvErr)
	}

	// Verify all keys present in receiver.
	iter, err := receiverDB.NewIter(nil)
	if err != nil {
		t.Fatalf("NewIter: %v", err)
	}
	defer iter.Close()

	count := 0
	for valid := iter.First(); valid; valid = iter.Next() {
		count++
	}

	if count < numKeys {
		t.Errorf("receiver has %d keys, want >= %d", count, numKeys)
	}

	// Spot check first and last.
	val, closer, err := receiverDB.Get([]byte("key-000000"))
	if err != nil {
		t.Fatalf("missing key-000000: %v", err)
	}
	if !bytes.Equal(val, []byte("value-000000")) {
		t.Errorf("key-000000: %q", val)
	}
	closer.Close()

	val, closer, err = receiverDB.Get([]byte(fmt.Sprintf("key-%06d", numKeys-1)))
	if err != nil {
		t.Fatalf("missing last key: %v", err)
	}
	if !bytes.Equal(val, []byte(fmt.Sprintf("value-%06d", numKeys-1))) {
		t.Errorf("last key: %q", val)
	}
	closer.Close()
}

func TestSnapshotSender_SnapshotSeq(t *testing.T) {
	db := newTestDB(t)
	repLog := NewReplicationLog(db)

	// Append entries to advance the seq.
	for i := 0; i < 10; i++ {
		if _, err := repLog.Append(OpSet, []byte("k"), []byte("v")); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	expectedSeq := repLog.CurrentSeq()
	if expectedSeq != 10 {
		t.Fatalf("expected seq=10, got %d", expectedSeq)
	}

	sender := NewSnapshotSender(db, repLog)
	senderConn, receiverConn := snapPipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	seqCh := make(chan uint64, 1)
	errCh := make(chan error, 1)
	go func() {
		seq, err := sender.Send(ctx, senderConn)
		seqCh <- seq
		errCh <- err
	}()

	// Read header to check seq in the wire format.
	headerType, headerPayload, err := receiverConn.Receive()
	if err != nil {
		t.Fatalf("receive header: %v", err)
	}
	if headerType != mbp.TypeSnapHeader {
		t.Fatalf("expected TypeSnapHeader, got 0x%02x", headerType)
	}

	var header mbp.SnapHeader
	if err := msgpack.Unmarshal(headerPayload, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header.SnapshotSeq != expectedSeq {
		t.Errorf("header.SnapshotSeq = %d, want %d", header.SnapshotSeq, expectedSeq)
	}

	// Ack and drain.
	if err := receiverConn.Send(mbp.TypeSnapAck, nil); err != nil {
		t.Fatalf("send ack: %v", err)
	}

	// Drain remaining frames.
	for {
		ft, _, err := receiverConn.Receive()
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if ft == mbp.TypeSnapComplete {
			break
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("sender error: %v", err)
	}
	returnedSeq := <-seqCh
	if returnedSeq != expectedSeq {
		t.Errorf("returned seq = %d, want %d", returnedSeq, expectedSeq)
	}
}

func TestSnapshotReceiver_WipeForResnapshot(t *testing.T) {
	// 1. Open a Pebble DB
	db := newTestDB(t)

	// 2. Write some keys directly to it
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("test-key-%03d", i))
		val := []byte(fmt.Sprintf("test-val-%03d", i))
		if err := db.Set(key, val, pebble.NoSync); err != nil {
			t.Fatalf("set key %d: %v", i, err)
		}
	}

	// Verify keys exist before wipe
	iter, err := db.NewIter(nil)
	if err != nil {
		t.Fatalf("NewIter before wipe: %v", err)
	}
	countBefore := 0
	for valid := iter.First(); valid; valid = iter.Next() {
		countBefore++
	}
	iter.Close()

	if countBefore != 100 {
		t.Errorf("before wipe: expected 100 keys, got %d", countBefore)
	}

	// 3. Call WipeForResnapshot()
	recv := NewSnapshotReceiver(db)
	if err := recv.WipeForResnapshot(); err != nil {
		t.Fatalf("WipeForResnapshot: %v", err)
	}

	// 4. Verify the DB is empty (iterate and count = 0)
	iter, err = db.NewIter(nil)
	if err != nil {
		t.Fatalf("NewIter after wipe: %v", err)
	}
	countAfter := 0
	for valid := iter.First(); valid; valid = iter.Next() {
		countAfter++
	}
	iter.Close()

	if countAfter != 0 {
		t.Errorf("after wipe: expected 0 keys, got %d", countAfter)
	}
}
