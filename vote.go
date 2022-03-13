package chatvotes

// VoteStore is used to keep track of votes. Votes have to be unique,
// meaning that multiple votes from the same entity should be ignored.
type VoteStore interface {
	AddUniqueVote(vote *Vote) bool
	Reset()
	GetVoteCount() int
	GetVotes() map[int]int
}

// Vote is issued by an entity.
type Vote struct {
	choice  int
	voterID string
}

// Voting is a snapshot of a finished voting.
type Voting struct {
	voteCount int
	votes     map[int]int
}

func (v *Voting) VoteCount() int {
	return v.voteCount
}

// Result returns the votes per option.
func (v *Voting) Result() map[int]int {
	return v.votes
}
