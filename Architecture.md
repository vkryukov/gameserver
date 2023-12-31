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


## Take 2: game should have NO starting position

After some thought, games should have NO staarting position. Of all the games of 
[GIPF project](https://en.wikipedia.org/wiki/GIPF_project),
only LYNGK has a randomized starting position (which we don't currently support, anyway):

1. [GIPF](https://en.wikipedia.org/wiki/GIPF_(game)) does have a starting position for Basic and 
   Standard GIPF, but it's always the  same, so knowing the game type is enough to restore it.

2. [ZERTZ](https://en.wikipedia.org/wiki/ZÈRTZ) players start with an empty board, but the board
   size might differ between a few predefined setups.

3. [DVONN](https://en.wikipedia.org/wiki/DVONN) players start with an empty board, and the game
   has two phases: placement and movement.

4. [YINSH](https://en.wikipedia.org/wiki/YINSH) players start with an empty board, and the game
   has two phases: placement and movement.

5. [PUNCT](https://en.wikipedia.org/wiki/PÜNCT) players start with an empty board.

6. [TZAAR](https://en.wikipedia.org/wiki/TZAAR) players either start with randomly filing the board,
   or with standard non-randomized position with initial placement.

7. LYNGK is the only game that requires a randomized starting position.

So in games 1-6, no starting position is needed. In games 1 and 6, we can include the standard
placement in the game's name, and in 2, do the same with the board size (e.g., ZERTZ 44). LYNGK
is the only tricky one - see the next section.


## For LYNGK and similar games, both playesr need to negotiate the starting position.

Indeed, if we define the starting position at the time of game *creation*, when only one
player has been identified so far, we cannot guarantee to the player who is joining that 
the starting position doesn't favor the first player. The idea is that, since both players
don't trust each other, once both are present, they each should contribute a piece of
randomness, that should be combined by the server to encode the starting position, which 
in this case can be encoded as a special `action_num = 0`. For example, both white and 
black send a permutations of numbers `1..n` and the server multiplies them to yield 
the final permutations that determines the start.

Again, no starting position is necessary here: just a special kind of negotations at the
game start.

Therefore, we'll remove all the starting position code from the codebase for now.