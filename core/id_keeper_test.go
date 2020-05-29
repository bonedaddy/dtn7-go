package core

import (
	"testing"

	"github.com/dtn7/dtn7-go/bundle"
)

func TestIdKeeper(t *testing.T) {
	bndl0, err := bundle.Builder().
		Source("dtn://src/").
		Destination("dtn://dest/").
		CreationTimestampEpoch().
		Lifetime("60s").
		BundleCtrlFlags(bundle.MustNotFragmented|bundle.RequestStatusTime).
		BundleAgeBlock(0, bundle.DeleteBundle).
		PayloadBlock([]byte("hello world!")).
		Build()
	if err != nil {
		t.Errorf("Creating bundle failed: %v", err)
	}

	bndl1, err := bundle.Builder().
		Source("dtn://src/").
		Destination("dtn://dest/").
		CreationTimestampEpoch().
		Lifetime("60s").
		BundleCtrlFlags(bundle.MustNotFragmented|bundle.RequestStatusTime).
		BundleAgeBlock(0, bundle.DeleteBundle).
		PayloadBlock([]byte("hello world!")).
		Build()
	if err != nil {
		t.Errorf("Creating bundle failed: %v", err)
	}

	var keeper = NewIdKeeper()

	keeper.update(&bndl0)
	keeper.update(&bndl1)

	if seq := bndl0.PrimaryBlock.CreationTimestamp.SequenceNumber(); seq != 0 {
		t.Errorf("First bundle's sequence number is %d", seq)
	}

	if seq := bndl1.PrimaryBlock.CreationTimestamp.SequenceNumber(); seq != 1 {
		t.Errorf("Second bundle's sequence number is %d", seq)
	}
}
