package registry

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/MysteriumNetwork/payments/registry/generated"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

//go:generate abigen --sol ../contracts/IdentityRegistry.sol --pkg generated --out generated/registry.go

type Registry struct {
	generated.IdentityRegistrySession
	Address common.Address
}

func DeployRegistry(owner *bind.TransactOpts, erc20address common.Address, backend bind.ContractBackend) (*Registry, error) {

	address, _, contract, err := generated.DeployIdentityRegistry(owner, backend, erc20address, big.NewInt(1000))
	if err != nil {
		return nil, err
	}

	return &Registry{
		generated.IdentityRegistrySession{
			TransactOpts: *owner,
			CallOpts:     bind.CallOpts{},
			Contract:     contract,
		},
		address,
	}, nil
}

func (registry *Registry) RegisterIdentity(data *RegistrationData) (*types.Transaction, error) {
	signature := data.Signature
	var pubKeyPart1 [32]byte
	var pubKeyPart2 [32]byte
	copy(pubKeyPart1[:], data.PublicKey.Part1)
	copy(pubKeyPart2[:], data.PublicKey.Part2)
	return registry.IdentityRegistrySession.RegisterIdentity(pubKeyPart1, pubKeyPart2, signature.V, signature.R, signature.S)
}

func (registry *Registry) LookupPublicKey(address common.Address) (*ecdsa.PublicKey, error) {
	part1, part2, err := registry.GetPublicKey(address)
	if err != nil {
		return nil, err
	}

	prefix := []byte{4}
	fullKey := append(prefix, append(part1[:], part2[:]...)...)
	return crypto.UnmarshalPubkey(fullKey)
}
