/*
 * Copyright (C) 2021 The "MysteriumNetwork/payments" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package transfer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog/log"
)

// Queue is channel queue which is able to
// queue transaction execution funcs.
type Queue struct {
	queue chan queueEntry
	stop  chan struct{}

	bc      QueueMultichainClient
	timeout time.Duration
	once    sync.Once
}

// ErrQueueClosed is returned when queue is closed and transaction was not processed.
var ErrQueueClosed = errors.New("queue was closed")

// ErrQueueMissingResult is returned if neither an error or a transaction exists in the queue response.
var ErrQueueMissingResult = errors.New("transaction missing with no previous error, state unknown")

// ErrTrasactionMissing is returned if a transaction was send to BC, but never found or confirmed.
var ErrTrasactionMissing = errors.New("transaction missing, state unknown")

type QueueMultichainClient interface {
	TransactionByHash(chainID int64, hash common.Hash) (*types.Transaction, bool, error)
}

type queueEntry struct {
	exec TransactionSendFn
	resp chan<- QueueResponse
}

// NewQueue returns a new queue. Size for the queue can be given
// so that more than 1 transaction could be queued at a time.
func NewQueue(size uint, cl QueueMultichainClient, timeout time.Duration) *Queue {
	return &Queue{
		queue:   make(chan queueEntry, size),
		bc:      cl,
		timeout: timeout,
		stop:    make(chan struct{}, 0),
	}
}

// Run will start to read the queue.
func (q *Queue) Run() {
	for {
		select {
		case <-q.stop:
			close(q.queue)
			for entry := range q.queue {
				q.resp(nil, ErrQueueClosed, entry.resp)
			}

			return
		case entry := <-q.queue:
			tx, err := entry.exec()
			if err != nil {
				q.resp(nil, err, entry.resp)
				continue
			}

			err = q.waitAfterSend(tx.ChainId().Int64(), tx.Hash())
			q.resp(tx, err, entry.resp)
		}
	}
}

func (q *Queue) waitAfterSend(chainID int64, txHash common.Hash) error {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()

	return q.checkIfTransactionExists(ctx, chainID, txHash)
}

func (q *Queue) checkIfTransactionExists(ctx context.Context, chainID int64, txHash common.Hash) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("transaction %q failed in queue: %w", txHash, ErrTrasactionMissing)
		case <-q.stop:
			return nil
		case <-time.After(time.Second):
			_, pending, err := q.bc.TransactionByHash(chainID, txHash)
			if err != nil {
				log.Err(err).Str("tx_hash", txHash.Hex()).Msg("failed to get transaction by hash in queue")
				continue
			}

			if !pending {
				return nil
			}
		}
	}
}

func (q *Queue) resp(tx *types.Transaction, err error, ch chan<- QueueResponse) {
	ch <- QueueResponse{
		tx:  tx,
		err: err,
	}
	close(ch)
}

// Stop will stop the thread. No new transactions can be enqueued after.
func (q *Queue) Stop() {
	q.once.Do(func() {
		close(q.stop)
	})
}

// TransactionEnqueue will enqueue a given transaction and respond on the given resp channel.
//
// Enqueue will fail and instantly return an error if the queue is closed.
// The given `resp` channel should be single use only. After receiving a message that channel will be closed.
func (q *Queue) TransactionEnqueue(exec TransactionSendFn, resp chan<- QueueResponse) error {
	select {
	case <-q.stop:
		// If stop is closed, dont submit new entries
		return ErrQueueClosed
	default:
		q.queue <- queueEntry{
			exec: exec,
			resp: resp,
		}
	}

	return nil
}

type QueueResponse struct {
	tx  *types.Transaction
	err error
}

// Result extracts the innards of the queue response.
func (qr *QueueResponse) Result() (*types.Transaction, error) {
	if qr.tx == nil && qr.err == nil {
		return nil, ErrQueueMissingResult
	}

	return qr.tx, qr.err
}
