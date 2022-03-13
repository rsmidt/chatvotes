package chatvotes

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

var _ VoteStore = (*StubVoteStore)(nil)

type StubVoteStore struct {
	votes      map[int]int
	voters     map[string]bool
	totalVotes int
}

func (s *StubVoteStore) GetVotes() map[int]int {
	return s.votes
}

func (s *StubVoteStore) GetVoteCount() int {
	return s.totalVotes
}

func (s *StubVoteStore) Reset() {
	s.voters = map[string]bool{}
	s.votes = map[int]int{}
}

func (s *StubVoteStore) AddUniqueVote(vote *Vote) bool {
	if s.voters[vote.voterID] {
		return false
	}
	s.voters[vote.voterID] = true
	s.votes[vote.choice]++
	s.totalVotes++
	return true
}

func newStubVoteStore() *StubVoteStore {
	store := &StubVoteStore{
		votes:  map[int]int{},
		voters: map[string]bool{},
	}
	return store
}

func TestPollSite_Start(t *testing.T) {
	t.Run("does not start voting if threshold is not reached within time", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 5,
			ReleaseTimeout: time.Millisecond * 5,
		})

		mustStartSilently(t, site)
		defer site.Stop()

		go func() {
			for i := 0; i < 5; i++ {
				vote := &Vote{
					voterID: fmt.Sprintf("voter %d", i),
					choice:  i,
				}
				time.Sleep(4 * time.Millisecond)
				site.InsertVote(vote)
			}
		}()

		assertTimeout(t, 12*time.Millisecond, func() {
			<-site.StateChanged()
		})
		if recordedVoteCount := store.votes[1]; recordedVoteCount != 0 {
			t.Errorf("expected no votes to be registered but got %d", recordedVoteCount)
		}
	})

	t.Run("starts voting if threshold is reached within time and respects votes before", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 5,
			ReleaseTimeout: time.Millisecond * 5,
		})

		mustStartSilently(t, site)
		defer site.Stop()

		go func() {
			for i := 0; i < 5; i++ {
				vote := &Vote{
					voterID: fmt.Sprintf("voter %d", i),
					choice:  1,
				}
				site.InsertVote(vote)
			}
		}()

		assertNoTimeout(t, 5*time.Millisecond, func() {
			assertStateChange(t, site, StateIdle, StateActiveVoting)
		})
		if recordedVoteCount := store.votes[1]; recordedVoteCount != 5 {
			t.Errorf("expected all %d votes to registered but got %d", 5, recordedVoteCount)
		}
	})

}

func TestPollSite_InsertVote(t *testing.T) {
	t.Run("returns an error if trying to insert votes on a stopped poll site", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 5,
			ReleaseTimeout: time.Millisecond * 5,
		})

		mustStartSilently(t, site)
		defer site.Stop()

		if err := site.InsertVote(&Vote{
			choice:  1,
			voterID: "asdf",
		}); err != nil {
			t.Error("expected inserting votes on a starting poll site to not fail")
		}
	})

	t.Run("inserting votes on a running poll site does not fail", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 5,
			ReleaseTimeout: time.Millisecond * 5,
		})

		if err := site.InsertVote(&Vote{
			choice:  1,
			voterID: "asdf",
		}); err != ErrNotStarted {
			t.Error("expected inserting votes on a stopped poll site to fail")
		}
	})
}

func TestPollSiteVotingFinished(t *testing.T) {
	t.Run("stops after timeout without votes", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 5,
			ReleaseTimeout: time.Millisecond * 5,
		})
		makeSiteWithVoting(t, site)
		defer site.Stop()

		timeWithoutVotes := time.Millisecond * 7
		assertNoTimeout(t, timeWithoutVotes, func() {
			assertStateChange(t, site, StateActiveVoting, StateIdle)
		})
	})

	t.Run("does not stop immediately if vote is registered", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 100,
			ReleaseTimeout: time.Millisecond * 50,
		})
		makeSiteWithVoting(t, site)
		defer site.Stop()

		go func() {
			site.InsertVote(&Vote{
				choice:  1,
				voterID: "foo",
			})
			time.Sleep(time.Millisecond * 40)
			site.InsertVote(&Vote{
				choice:  1,
				voterID: "foo2",
			})
		}()

		timeWithoutVotes := time.Millisecond * 40
		assertTimeout(t, timeWithoutVotes, func() {
			<-site.StateChanged()
		})
	})

	t.Run("resets votes after returning to idle", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 25,
			ReleaseTimeout: time.Millisecond * 10,
		})
		makeSiteWithVoting(t, site)
		defer site.Stop()

		timeWithoutVotes := time.Millisecond * 40
		assertNoTimeout(t, timeWithoutVotes, func() {
			assertStateChange(t, site, StateActiveVoting, StateIdle)
		})
		assertEmptyStubStore(t, store)
	})

	t.Run("released site can start voting again", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 25,
			ReleaseTimeout: time.Millisecond * 10,
		})
		makeSiteWithVoting(t, site)
		defer site.Stop()

		timeWithoutVotes := time.Millisecond * 40
		assertNoTimeout(t, timeWithoutVotes, func() {
			assertStateChange(t, site, StateActiveVoting, StateIdle)
		})
		assertEmptyStubStore(t, store)

		go func() {
			for i := 0; i < site.config.StartThreshold; i++ {
				vote := &Vote{
					voterID: fmt.Sprintf("voter %d", i),
					choice:  1,
				}
				site.InsertVote(vote)
			}
		}()

		assertNoTimeout(t, site.config.StartTimeout+time.Millisecond, func() {
			assertStateChange(t, site, StateIdle, StateActiveVoting)
		})
	})

	t.Run("returns a result after finishing a voting", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 25,
			ReleaseTimeout: time.Millisecond * 10,
		})
		makeSiteWithVoting(t, site)
		defer site.Stop()

		// By the time, we read the store in the assertions the store will already be reset.
		votesBeforeRelease := store.votes

		timeWithoutVotes := time.Millisecond * 40
		assertNoTimeout(t, timeWithoutVotes, func() {
			voting := <-site.VotingFinished()
			if voteCount := voting.VoteCount(); voteCount != 5 {
				t.Errorf("expected vote count to be %d but got %d", 5, voteCount)
			}
			if result := voting.Result(); !reflect.DeepEqual(result, votesBeforeRelease) {
				t.Errorf("result of voting (%+v) does not match store state (%+v)", result, store.votes)
			}
		})
	})

	t.Run("allows to be cancelled with context", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 25,
			ReleaseTimeout: time.Millisecond * 10,
		})
		defer site.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(time.Millisecond * 2)
			cancel()
		}()

		assertNoTimeout(t, time.Millisecond*5, func() {
			if err := site.Start(ctx); err == nil {
				t.Fatal("expected cancelled poll site to return error")
			}
		})
	})

	t.Run("does not return error if stopped normally", func(t *testing.T) {
		store := newStubVoteStore()
		site := NewPollSite(store, &PollSiteConfig{
			StartThreshold: 5,
			StartTimeout:   time.Millisecond * 25,
			ReleaseTimeout: time.Millisecond * 10,
		})

		go func() {
			time.Sleep(time.Millisecond * 2)
			site.Stop()
		}()

		assertNoTimeout(t, time.Millisecond*5, func() {
			if err := site.Start(context.Background()); err != nil {
				t.Fatalf("expected stopping poll site to not return error: %v", err)
			}
		})
	})
}

func assertEmptyStubStore(t *testing.T, store *StubVoteStore) {
	t.Helper()
	if len(store.votes) != 0 {
		t.Error("expected store votes to be reset after release")
	}
	if len(store.voters) != 0 {
		t.Error("expected store voters to be reset after release")
	}
}

func makeSiteWithVoting(t *testing.T, site *PollSite) {
	t.Helper()
	mustStartSilently(t, site)

	go func() {
		for i := 0; i < site.config.StartThreshold; i++ {
			vote := &Vote{
				voterID: fmt.Sprintf("voter %d", i),
				choice:  1,
			}
			site.InsertVote(vote)
		}
	}()

	assertNoTimeout(t, site.config.StartTimeout+time.Millisecond, func() {
		assertStateChange(t, site, StateIdle, StateActiveVoting)
	})
}

func mustStartSilently(t *testing.T, site *PollSite) {
	t.Helper()
	go func() {
		time.Sleep(time.Millisecond)
		site.Start(context.Background())
	}()
	requireNoTimeout(t, time.Millisecond*20, func() {
		assertStateChange(t, site, StateStopped, StateIdle)
	})
}

func assertStateChange(t *testing.T, site *PollSite, expectedPrevState PollSiteState, expectedNextState PollSiteState) {
	t.Helper()
	trans := <-site.StateChanged()
	if trans.From != expectedPrevState {
		t.Fatalf("expected prev state to be %s, got %s", expectedPrevState, trans.From)
	}
	if trans.To != expectedNextState {
		t.Fatalf("expected next state to be %s, got %s", expectedNextState, trans.To)
	}
}

func assertTimeout(t *testing.T, timeout time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	start := time.Now()

	go func() {
		f()
		done <- struct{}{}
	}()

	select {
	case <-done:
		end := time.Since(start)
		t.Errorf("function did not time out after %+v, instead it returned after %+v", timeout, end)
	case <-time.After(timeout):
	}
}

func assertNoTimeout(t *testing.T, timeout time.Duration, f func()) {
	t.Helper()
	timeoutFunc(t, timeout, f, func() {
		t.Helper()
		t.Errorf("function timed out after %s", timeout)
	})
}

func requireNoTimeout(t *testing.T, timeout time.Duration, f func()) {
	t.Helper()
	timeoutFunc(t, timeout, f, func() {
		t.Helper()
		t.Fatalf("function timed out after %s", timeout)
	})
}

func timeoutFunc(t *testing.T, timeout time.Duration, subject, onTimeout func()) {
	t.Helper()
	done := make(chan struct{})

	go func() {
		subject()
		done <- struct{}{}
	}()

	select {
	case <-time.After(timeout):
		onTimeout()
	case <-done:
	}
}
