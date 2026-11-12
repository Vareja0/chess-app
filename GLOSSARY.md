# Chess App

Online two-player chess: players are matched by time control and play a live game in a shared room.

## Language

**Game**:
One chess game between two players: its position, moves, clocks and eventual result.
_Avoid_: Match, session

**Room**:
The live home of exactly one Game, where both players connect while it is played.
_Avoid_: Lobby, channel, table

**Clock**:
A player's remaining thinking time in a Game; only the side to move has a running Clock.
_Avoid_: Timer

**Time control**:
The starting time each Clock gets: bullet, blitz, rapid or classical.
_Avoid_: Time mode, game mode

**Abort**:
Ending a Game before both sides have made their first move; it has no winner and counts for neither player.
_Avoid_: Cancel (cancel refers to leaving the matchmaking queue)

**Draw offer**:
One player's standing proposal to end the Game in a draw; the opponent accepts or declines it. A player may make one per move of theirs, and moving withdraws it.
_Avoid_: Draw request, draw proposal, agreement (agreement is the Result method, not the offer)

**Result**:
How a finished Game ended: the winner (white, black, or none for a draw) and the method (checkmate, resign, timeout, disconnect, stalemate, agreement, and so on).
_Avoid_: Outcome, score
