package chatvotes

import (
	"context"
	"errors"
	"time"
)

// ErrNotStarted is used to indicate that the poll site is not started.
var ErrNotStarted = errors.New("poll site is not started")

// PollSiteState represents the state of a PollSite.
type PollSiteState int

func (p PollSiteState) String() string {
	switch p {
	case StateIdle:
		return "Idle"
	case StateActiveVoting:
		return "ActiveVoting"
	case StateStopped:
		return "Stopped"
	}
	return ""
}

const (
	// StateStopped represents a stopped poll site. A poll site always starts in this state.
	// It can only be transitioned away from by starting the poll site.
	StateStopped PollSiteState = iota

	// StateIdle represents a poll site that can receive votes but has not yet reached the threshold
	// to start a voting.
	StateIdle

	// StateActiveVoting represents a poll site that currently associates all incoming votes
	// with a specific voting. After a release condition, the poll site will transition back
	// to StateIdle.
	StateActiveVoting
)

// StateTransition represents a transition from one PollSiteState to another.
type StateTransition struct {
	From PollSiteState
	To   PollSiteState
}

// PollSiteConfig is the configuration for a poll site.
type PollSiteConfig struct {
	// startTimeout is the maximum duration in which the required vote count
	// specified in startThreshold has to be reached.
	startTimeout time.Duration

	// startThreshold is the minimum number of votes that to has to be reached
	// before the start times out.
	startThreshold int

	// releaseTimeout is the maximum duration in which votes have to be registered
	// to reset the release timeout and keep the voting alive.
	// After reaching the timeout, the poll site will transition back to StateIdle.
	releaseTimeout time.Duration
}

// NewPollSite creates and sets up a new PollSite.
func NewPollSite(store VoteStore, p *PollSiteConfig) *PollSite {
	return &PollSite{
		store:           store,
		stateChanged:    make(chan StateTransition),
		state:           StateStopped,
		incomingVotes:   make(chan *Vote, 5),
		done:            make(chan struct{}),
		finishedVotings: make(chan Voting),
		config:          p,
	}
}

// PollSite is a manager that processes incoming votes and decides based on PollSiteConfig
// whether votes received are enough to be considered a voting.
type PollSite struct {
	store         VoteStore
	incomingVotes chan *Vote
	done          chan struct{}
	config        *PollSiteConfig
	voteCache     []*Vote

	stateChanged chan StateTransition
	state        PollSiteState

	startTicker     *time.Ticker
	releaseTicker   *time.Ticker
	finishedVotings chan Voting
}

// InsertVote tries to insert a vote.
// Returns ErrNotStarted if the poll site has not yet been started.
func (ps *PollSite) InsertVote(vote *Vote) error {
	if ps.state == StateStopped {
		return ErrNotStarted
	}
	ps.incomingVotes <- vote
	return nil
}

// Stop stops the poll site and prevents if from further processing votes.
func (ps *PollSite) Stop() {
	ps.setNextState(StateStopped)
	close(ps.done)
}

// Start starts the poll site blocking until Stop is called
// or the context is cancelled.
// Returns an error on context cancellation explaining why.
func (ps *PollSite) Start(ctx context.Context) error {
	ps.setNextState(StateIdle)

	ps.startTicker = time.NewTicker(ps.config.startTimeout)
	ps.releaseTicker = time.NewTicker(ps.config.releaseTimeout)

	for {
		select {
		case vote := <-ps.incomingVotes:
			ps.handleNewVote(vote)
		case <-ps.startTicker.C:
			ps.handleStartTimeout()
		case <-ps.releaseTicker.C:
			ps.handleReleaseTimeout()
		case <-ctx.Done():
			return ctx.Err()
		case <-ps.done:
			return nil
		}
	}
}

// StateChanged publishes all stage changes.
// This is useful to determine if a voting has started.
func (ps *PollSite) StateChanged() <-chan StateTransition {
	return ps.stateChanged
}

// VotingFinished publishes all finished votings.
func (ps *PollSite) VotingFinished() <-chan Voting {
	return ps.finishedVotings
}

func (ps *PollSite) handleStartTimeout() {
	if ps.state != StateIdle {
		return
	}
	ps.voteCache = nil
}

func (ps *PollSite) handleNewVote(vote *Vote) {
	switch ps.state {
	case StateIdle:
		ps.voteCache = append(ps.voteCache, vote)
		if len(ps.voteCache) < ps.config.startThreshold {
			return
		}

		for _, v := range ps.voteCache {
			ps.store.AddUniqueVote(v)
		}
		ps.voteCache = nil
		ps.releaseTicker.Reset(ps.config.releaseTimeout)
		ps.setNextState(StateActiveVoting)
	case StateActiveVoting:
		ps.store.AddUniqueVote(vote)

		ps.releaseTicker.Reset(ps.config.releaseTimeout)
	}
}

func (ps *PollSite) handleReleaseTimeout() {
	if ps.state != StateActiveVoting {
		return
	}

	select {
	case ps.finishedVotings <- Voting{
		voteCount: ps.store.GetVoteCount(),
		votes:     ps.store.GetVotes(),
	}:
	default:
	}

	ps.store.Reset()
	ps.startTicker.Reset(ps.config.releaseTimeout)
	ps.setNextState(StateIdle)
}

func (ps *PollSite) setNextState(state PollSiteState) {
	fromState := ps.state
	ps.state = state
	select {
	case ps.stateChanged <- StateTransition{
		From: fromState,
		To:   state,
	}:
	default:
	}
}
