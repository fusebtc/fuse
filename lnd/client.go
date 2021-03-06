package lnd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/mdedys/fuse/lightning"
)

var (
	NullPreimage          = lntypes.Preimage{}
	ErrInvoiceAlreadyPaid = errors.New("invoice has already been paid")
)

type lnd interface {
	AddInvoice(ctx context.Context, in *invoicesrpc.AddInvoiceData) (lntypes.Hash, string, error)
	PayInvoice(ctx context.Context, invoice string, maxFee btcutil.Amount, outgoingChannel *uint64) chan lndclient.PaymentResult
	WalletBalance(ctx context.Context) (*lndclient.WalletBalance, error)

	Connect(ctx context.Context, peer route.Vertex, host string, permanent bool) error
	ListPeers(ctx context.Context) ([]lndclient.Peer, error)
	OpenChannel(ctx context.Context, peer route.Vertex, localSat, pushSat btcutil.Amount, private bool) (*wire.OutPoint, error)
	ListChannels(ctx context.Context, activeOnly, publicOnly bool) ([]lndclient.ChannelInfo, error)
}

type LndClient struct {
	lnd     lnd
	network lightning.Network
	maxFee  btcutil.Amount
}

func (c LndClient) WalletBalance(ctx context.Context) (btcutil.Amount, error) {
	balance, err := c.lnd.WalletBalance(ctx)
	if err != nil {
		return btcutil.Amount(0), err
	}
	return balance.Confirmed, nil
}

func (c LndClient) PayInvoice(ctx context.Context, invoice lightning.Invoice) (lightning.PaymentResult, error) {
	result := <-c.lnd.PayInvoice(ctx, invoice.Encoded, c.maxFee, nil)
	if result.Err != nil {
		return lightning.PaymentResult{}, result.Err
	}

	if result.Preimage == NullPreimage {
		return lightning.PaymentResult{}, ErrInvoiceAlreadyPaid
	}

	return lightning.PaymentResult{PreImage: result.Preimage, PaidFee: result.PaidFee}, nil
}

func (c LndClient) AddInvoice(ctx context.Context, value lnwire.MilliSatoshi, memo string, hhash []byte) (lightning.Invoice, error) {

	data := &invoicesrpc.AddInvoiceData{Value: value}
	if len(hhash) > 0 {
		data.DescriptionHash = hhash
	} else {
		data.Memo = memo
	}

	_, encoded, err := c.lnd.AddInvoice(ctx, data)
	if err != nil {
		return lightning.Invoice{}, err
	}

	invoice, err := lightning.DecodeInvoice(encoded, c.network)
	if err != nil {
		return lightning.Invoice{}, err
	}

	return invoice, nil
}

func (c LndClient) ListPeers(ctx context.Context) ([]lightning.Peer, error) {
	lndPeers, err := c.lnd.ListPeers(ctx)
	if err != nil {
		return []lightning.Peer{}, err
	}

	var peers []lightning.Peer

	for _, p := range lndPeers {
		peers = append(peers, lightning.Peer{
			Address:  p.Address,
			Inbound:  p.Inbound,
			PingTime: p.PingTime,
			Pubkey:   lightning.Vertex(p.Pubkey),
			Sent:     p.Sent,
			Received: p.Received,
		})
	}

	return peers, nil
}

func (c LndClient) ConnectPeer(ctx context.Context, peer lightning.Vertex, host string) error {
	return c.lnd.Connect(ctx, route.Vertex(peer), host, true)
}

func (c LndClient) OpenChannel(ctx context.Context, peer lightning.Vertex, localSat, pushSat btcutil.Amount, private bool) (chainhash.Hash, uint32, error) {
	result, err := c.lnd.OpenChannel(ctx, route.Vertex(peer), localSat, pushSat, private)
	if err != nil {
		return chainhash.Hash{}, 0, err
	}
	return result.Hash, result.Index, nil
}

func (c LndClient) ListChannels(ctx context.Context, activeOnly, publicOnly bool) ([]lightning.Channel, error) {
	lndChannels, err := c.lnd.ListChannels(ctx, activeOnly, publicOnly)
	if err != nil {
		return []lightning.Channel{}, err
	}

	var channels []lightning.Channel
	for _, c := range lndChannels {
		channels = append(channels, lightning.Channel{
			ID:            c.ChannelID,
			Capacity:      c.Capacity,
			LocalBalance:  c.LocalBalance,
			RemoteBalance: c.RemoteBalance,
			Active:        c.Active,
			Private:       c.Private,
			RemotePubkey:  lightning.Vertex(c.PubKeyBytes),
		})
	}

	return channels, nil
}

func connect(address, network, macPath, tlsPath string) (lndclient.LightningClient, error) {

	cfg := &lndclient.LndServicesConfig{
		LndAddress:         address,
		Network:            lndclient.Network(network),
		CustomMacaroonPath: macPath,
		TLSPath:            tlsPath,
	}

	var lnd lndclient.LightningClient
	err := retry.Do(
		func() error {
			services, err := lndclient.NewLndServices(cfg)
			if err != nil {
				fmt.Printf("Failed to connect to LND: %s", err.Error())
				return err
			}
			lnd = services.Client
			return nil
		},
		retry.Attempts(10),
		retry.Delay(time.Duration(1)*time.Second),
	)

	return lnd, err
}

func NewClient(address, network, macPath, tlsPath string, maxFee btcutil.Amount) (*LndClient, error) {
	lnd, err := connect(address, network, macPath, tlsPath)
	if err != nil {
		return nil, err
	}
	return &LndClient{lnd: lnd, network: lightning.Network(network), maxFee: maxFee}, err
}
