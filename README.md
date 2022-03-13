# :bar_chart: chatvotes

See what your chat really wants without lifting a finger.

## About

Often in streamer chats you will see spam like this:
 
```
user1: 1
user2: 2
user3: 1
user4: 1
user5: 2
...
```

Streamers may use this for their decision-making (e.g. which dialogue option to choose).
But just glancing over the chat is not really democratic hence we
need something to give everyone an exact picture. This is where chatvotes come in.

## Installation

`go get -u github.com/rsmidt/chatvotes`

## Quick Start

A `PollSite` is the starting point for registering votes.
You can configure the start and release timeouts and threhshold
with `PollSiteConfig`:

```go
site := chatvotes.NewPollSite(voteStore, &chatvotes.PollSiteConfig{
    startTimeout: time.Seconds * 10,
	startThreshold: 15,
	releaseTimeout: time.Seconds * 20
})

defer site.Stop()
go site.Start(context.TODO())

site.InsertVote(...)

voting := <-site.VotingFinished()
voting.VoteCount()
voting.Result()
```

This means that if within 10 seconds the poll site receives
at least 15 votes, a formal voting will be started.
After 20 seconds without a vote, the voting will be closed
and a message is published via the `VotingFinished` channel.

To get informed when a voting starts (e.g. to send live updates to the chat),
you can use the `StateChanged` channel:

```go
transition := <-site.StateChanged()
if transition.to == chatvotes.StateActiveVoting {
    // e.g. send message to chat or start live update interval.
}
```

## Status / Roadmap

This is very early software and I would not recommend using it by now.
Currently, it's missing

* `VoteStore` implementations: at least an in-memory store
* adapters for different chat systems: IRC, Twitch, YouTube (should this be part of this package?)
* more fine-grained rules for voting detection (e.g. based on chat frequency / number of chatters etc.)
* different types of choices for votes (e.g. not only numbers but maybe words that spammed a lot)

