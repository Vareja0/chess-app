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

**Result**:
How a finished Game ended: the winner (white, black, or none for a draw) and the method (checkmate, resign, timeout, disconnect, stalemate, and so on).
_Avoid_: Outcome, score
