// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/gecko/ids"
	"github.com/ava-labs/gecko/utils/logging"
)

// TODO: There is a conservative early termination case that doesn't require dag
// traversals we may want to implement. The algorithm would go as follows:
// Keep track of the number of response that reference an ID. If an ID gets >=
// alpha responses, then remove it from all responses and place it into a chit
// list. Remove all empty responses. If the number of responses + the number of
// pending responses is less than alpha, terminate the poll.
// In the synchronous + virtuous case, when everyone returns the same hash, the
// poll now terminates after receiving alpha responses.
// In the rogue case, it is possible that the poll doesn't terminate as quickly
// as possible, because IDs may have the alpha threshold but only when counting
// transitive votes. In this case, we may wait even if it is no longer possible
// for another ID to earn alpha votes.
// Because alpha is typically set close to k, this may not be performance
// critical. However, early termination may be performance critical with crashed
// nodes.

type polls struct {
	log      logging.Logger
	numPolls prometheus.Gauge
	m        map[uint32]poll
}

// Add to the current set of polls
// Returns true if the poll was registered correctly and the network sample
//         should be made.
func (p *polls) Add(requestID uint32, numPolled int) bool {
	poll, exists := p.m[requestID]
	if !exists {
		poll.numPending = numPolled
		p.m[requestID] = poll

		p.numPolls.Set(float64(len(p.m))) // Tracks performance statistics
	}
	return !exists
}

// Vote registers the connections response to a query for [id]. If there was no
// query, or the response has already be registered, nothing is performed.
func (p *polls) Vote(requestID uint32, vdr ids.ShortID, votes []ids.ID) (ids.UniqueBag, bool) {
	p.log.Verbo("Vote. requestID: %d. validatorID: %s.", requestID, vdr)
	poll, exists := p.m[requestID]
	p.log.Verbo("Poll: %+v", poll)
	if !exists {
		return nil, false
	}

	poll.Vote(votes)
	if poll.Finished() {
		p.log.Verbo("Poll is finished")
		delete(p.m, requestID)
		p.numPolls.Set(float64(len(p.m))) // Tracks performance statistics
		return poll.votes, true
	}
	p.m[requestID] = poll
	return nil, false
}

func (p *polls) String() string {
	sb := strings.Builder{}

	sb.WriteString(fmt.Sprintf("Current polls: (Size = %d)", len(p.m)))
	for requestID, poll := range p.m {
		sb.WriteString(fmt.Sprintf("\n    %d: %s", requestID, poll))
	}

	return sb.String()
}

// poll represents the current state of a network poll for a vertex
type poll struct {
	votes      ids.UniqueBag
	numPending int
}

// Vote registers a vote for this poll
func (p *poll) Vote(votes []ids.ID) {
	if p.numPending > 0 {
		p.numPending--
		p.votes.Add(uint(p.numPending), votes...)
	}
}

// Finished returns true if the poll has completed, with no more required
// responses
func (p poll) Finished() bool { return p.numPending <= 0 }
func (p poll) String() string { return fmt.Sprintf("Waiting on %d chits", p.numPending) }
