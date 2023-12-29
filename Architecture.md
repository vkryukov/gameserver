## Games should have starting position, separate from list of actions

In our current implementation, each game is just a list of actions. This design can support
any starting position: just encode it as a sequence of actions. This approach works fine 
for GIPF, where the non-empty starting positions (for Basic and Standard GIFP games) can
be thought of as a sequence of moves by white and black player.

This won't work with any game of the GIPF project though. For example, LYNGK requires a
randomized starting board; leaving aside that the server currently doesn't support this
kind of setup, "populating the starting board with pieces" does not naturally fit into
"actions done by players" design; especially given that in the setup phase, these "actions"
are pretty different from the regular game.

A solution therefore is to store the starting position separately from the list of actions made
by players.

This also solves another problem in the current design: each action should be signed by a player,
to make sure that other clients can verify that the move was made by whoever claims they made it.
The design doesn't extend naturally to starting positions, so storing them separately addresses
this particular concern.