package lightning

import (
	"context"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil"
	"github.com/lightningnetwork/lnd/lnwire"
)

type LightningProvider interface {
	// Balance
	WalletBalance(ctx context.Context) (btcutil.Amount, error)

	// Invoices
	AddInvoice(ctx context.Context, value lnwire.MilliSatoshi, memo string, hhash []byte) (Invoice, error)
	PayInvoice(ctx context.Context, invoice Invoice) (PaymentResult, error)

	// Peers
	ListPeers(ctx context.Context) ([]Peer, error)
	ConnectPeer(ctx context.Context, peer Vertex, host string) error

	// Channels
	OpenChannel(ctx context.Context, peer Vertex, localSat, pushSat btcutil.Amount, private bool) (chainhash.Hash, uint32, error)
	ListChannels(ctx context.Context, active, public bool) ([]Channel, error)
}

type LightningClient struct {
	provider LightningProvider
}

func New(provider LightningProvider) *LightningClient {
	client := LightningClient{provider}
	return &client
}
