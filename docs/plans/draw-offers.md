# Draw offers

## Goal

A player may offer a draw during a Game. The opponent accepts or declines. Accepting ends the
Game with no winner and the method `agreement`.

Full vertical slice: the `game` package, the Room's websocket protocol, and the browser UI.

## Out of scope

- A JS test harness. The repo has none; the UI slice is verified by hand.
- An explicit "withdraw offer" command. Moving withdraws the offer.
- Time-based expiry of an offer, draw-offer rate limits beyond the one-per-move rule below,
  and any persistence of offers (an offer lives only in the in-memory Game).

## Decisions

| # | Decision | Why |
|---|---|---|
| 1 | Offers are allowed at **any time** while the Game is in progress, on either side's turn, including before both sides have made their first move. | Matches how online servers behave; no reason to make the player wait for their own turn. |
| 2 | An offer is **withdrawn when the offerer moves**. The opponent's move leaves it standing. | Ties the offer to the position it was made in without needing a timer. The opponent may think, then accept. |
| 3 | **One offer per own move**: after offering, a player may not offer again until they have made a move — whether their offer was declined or withdrawn. | Stops offer spam with a single invariant rather than a cooldown clock. |
| 4 | Offering while the **opponent's offer is open counts as accepting it**. | Both sides have said "draw"; rejecting on a technicality would be surprising. |
| 5 | No explicit withdraw command. | The brief asks for a pair of commands; decision 2 already covers changing your mind. |
| 6 | Both the pending offer **and the asking player's right to offer** are Game state, replayed in the `joined` payload as `draw_offer` and `can_offer`. | A reconnecting client's UI must agree with the server. Replaying only the offer would leave the client rendering an enabled Draw button in exactly the state where decision 3 makes offering illegal. |
| 7 | Every draw event is **broadcast to both players** with a `from` color. Each client decides from `from` whether the event is about them. | Both clients then hold identical offer state, which is what decision 6's replay has to agree with. |
| 8 | Withdrawal is announced with its **own `draw_withdrawn` event**, not inferred from the `move` broadcast. | The "whose offer, did they just move" rule stays on the server instead of being reimplemented per client. |
| 9 | Accepting **charges the side to move** for their thinking time before freezing the Clocks, exactly as `Resign` and `Disconnect` do (`game/game.go:177-185`). | The stored and broadcast Clock snapshot must reflect the time actually spent. |
| 10 | `settle` keeps priority: an open offer neither blocks `Abort` nor stops the 30 s auto-Abort window or a Clock running out. | Consistent with how Resign and Disconnect already lose to a fallen flag (`TestGame_EndsExactlyOnce`). |
| 11 | `State.DrawOffer` reads `NoColor` once the Game has ended, whatever ended it. | A finished Game has no live offer. Leaving the field set would make callers ask whether the Result or the offer wins. |
| 12 | A `draw_response` with no `accept` field **declines** (Go's zero value). | Declining is the safe default: a malformed message must never end a Game. Documented so it is a decision rather than an accident. |
| 13 | Glossary gains **Draw offer**; the Result entry, the README feature list and the README protocol block gain the new messages, codes and method. | `README.md:94-113` is the canonical protocol reference and would otherwise silently misdocument this change. |
| 14 | While an offer stands **against** you, `drawBtn` is **disabled** — the modal's Aceitar/Recusar is the only way to respond. Offering a draw never asks for confirmation. | Decision 4 makes a click on "Draw" end the Game instantly in that state, with no confirm, unlike Resign (`indexScript.js:509-515`). Disabling removes the trap without inventing a second label for one button. Decision 4 still holds on the wire for any other client. |
| 15 | The draw prompt has **no outside-click dismiss**. It closes only on Aceitar, Recusar, or a server event. | The `.promo-overlay` pattern dismisses on outside click (`indexScript.js:79-84`). Copying that would hide an offer that is still open server-side, with nothing to re-send it. |
| 16 | The prompt is a **non-blocking card** beside the board (`.draw-offer`, no `inset: 0`, no backdrop blur), not a full-screen overlay. *Added after code review.* | A blocking overlay hides the position while the recipient's Clock runs, and makes decision 2's own rationale — "the opponent may think, then accept" — and behavior G7 unreachable from the UI. The server already lets the recipient move with the offer standing; the UI must not take that away. |

### Derived from the above

- `Moved` gains `DrawWithdrawn bool` so the Room knows a move lapsed an offer and can emit
  `draw_withdrawn` (decision 8). It is the event the Room must relay, so the Room should not have
  to diff `State` before and after the move to find it.
- The offering right is per-player, so it is exposed as a method, `CanOfferDraw(color, now)`,
  rather than a field on the shared `State` snapshot.
- In `OfferDraw`, the **mutual-accept check (decision 4) runs before the one-per-move check
  (decision 3)**. A player whose offering right is spent, who then offers into the opponent's open
  offer, still draws the Game. Accept must never lose to a spam guard (G9).
- Both accept paths — `RespondToDraw(accept: true)` and `OfferDraw` into an open offer — go through
  one unexported `acceptDraw(now)` helper that charges the side to move and then ends the Game, so
  the two paths cannot drift.
- `ErrDrawAlreadyOffered` also covers the "you offered, were declined, and have not moved since"
  case. The name describes the common case; the doc comment states the full rule.

## Public interfaces

### `game` package (`game/game.go`)

```go
const Agreement Method = "agreement"

var (
    // ErrDrawAlreadyOffered means the player has already offered a draw since their last move.
    ErrDrawAlreadyOffered = errors.New("draw already offered")
    // ErrNoDrawOffer means there is no draw offer from the opponent to respond to.
    ErrNoDrawOffer = errors.New("no draw offer to respond to")
)

// OfferDraw offers a draw to color's opponent. If the opponent already had an offer open, it is
// accepted and the Game ends in a draw by agreement. A player may offer only once per move of
// theirs: offering again before moving returns ErrDrawAlreadyOffered.
// The returned Result is nil unless this call ended the Game.
func (g *Game) OfferDraw(color Color, now time.Time) (*Result, error)

// RespondToDraw accepts or declines the offer standing against color. Accepting ends the Game in a
// draw by agreement; declining leaves it in progress, and the offerer may not offer again until
// they move. With no offer against color it returns ErrNoDrawOffer.
// The returned Result is nil unless this call ended the Game.
func (g *Game) RespondToDraw(color Color, accept bool, now time.Time) (*Result, error)

// CanOfferDraw reports whether color may offer a draw now: false once they have offered and not
// yet moved, and false once the Game has ended.
func (g *Game) CanOfferDraw(color Color, now time.Time) bool
```

Changed structs:

```go
type Moved struct {
    FEN           string
    Turn          Color
    Clocks        Clocks
    Result        *Result
    DrawWithdrawn bool // the mover had an offer open, and this move withdrew it
}

type State struct {
    FEN       string
    Moves     []string
    Turn      Color
    Clocks    Clocks
    Result    *Result
    DrawOffer Color // who has an offer open; NoColor for none, and for a finished Game
}
```

Internal state on `Game`: `drawOffer Color` and `canOffer map[Color]bool` (both sides start able to
offer; `Move` restores the mover's right and clears the mover's own open offer).

### Wire protocol (`controllers/gameController.go`)

Client → server:

```json
{"type": "draw_offer"}
{"type": "draw_response", "accept": true}
```

`clientMessage` gains `Accept bool \`json:"accept"\``.

Server → both players:

| Message | Sent when |
|---|---|
| `{"type":"draw_offered","from":"white"}` | an offer opens |
| `{"type":"draw_declined","from":"black"}` | the opponent declines (`from` is the responder) |
| `{"type":"draw_withdrawn","from":"white"}` | the offerer moved, lapsing their offer |
| existing `game_over` with `"winner":null,"method":"agreement"` | the offer is accepted |

Server → sender only (existing `error` shape): codes `draw_already_offered`
("Você já ofereceu empate neste lance") and `no_draw_offer` ("Nenhuma oferta de empate pendente").
`ErrGameOver` gets no reply, as with the existing commands.

The broadcast conditions are predicates, not prose — a `RespondToDraw(accept: true)` must never also
emit `draw_declined`:

| Broadcast | Predicate |
|---|---|
| `draw_offered` | `OfferDraw` returned `err == nil && result == nil` |
| `draw_declined` | `RespondToDraw` returned `err == nil && result == nil` (which implies `!msg.Accept`) |
| `draw_withdrawn` | `Move` returned `err == nil && moved.DrawWithdrawn`, sent after the `move` broadcast |

`joinedMessage` gains:

- `DrawOffer *string \`json:"draw_offer"\`` — null when none, encoded with the existing `colorJSON`;
- `CanOffer bool \`json:"can_offer"\`` — `game.CanOfferDraw(cmd.color, now)` for the joining player.

Accepting needs no explicit broadcast: the Room's loop sees a non-nil `Result` and `finish` sends
`game_over` and persists the record, exactly as for Resign.

### UI (`views/index.tmpl`, `public/scripts/indexScript.js`)

- Match-actions grid becomes `grid-cols-3`; a `drawBtn` joins Abort and Resign, same button classes,
  label "Draw", icon `handshake`. Clicking it sends `{"type":"draw_offer"}` with no `confirm()`
  (decision 14).
- Client keeps two pieces of state, `let canOffer = false; let offerAgainstMe = false;`, seeded from
  `joined.can_offer` and `joined.draw_offer`. `drawBtn` carries the hardcoded `disabled` attribute
  in the template, like `abortBtn` and `resignBtn` (`views/index.tmpl:179,187`), so the pre-join
  state matches. A single `updateDrawBtn()` helper, mirroring the existing `updateAbortBtn()`,
  applies:

  ```js
  drawBtn.disabled = gameOver || !canOffer || offerAgainstMe || ws?.readyState !== WebSocket.OPEN;
  ```

  Call sites: `joined`, `move`, `draw_offered`, `draw_declined`, `draw_withdrawn`, `game_over`,
  the `drawBtn` click handler, and `ws.onclose` (which already disables `resignBtn` and `abortBtn`,
  `indexScript.js:306-307`).
- `canOffer` is cleared **optimistically in the click handler** as well as on the player's own
  `draw_offered`, so a double-click cannot fire two offers and earn a spurious
  "Você já ofereceu empate neste lance". It is restored on the echo of the player's own `move`.
- **Color encoding**: `myColor` is a single char (`'w'`/`'b'`, `indexScript.js:238`) while `from`
  and `turn` on the wire are `"white"`/`"black"`. A bare `data.from !== myColor` is always true and
  would pop the modal at the offerer. Compare on the first character:

  ```js
  const fromMe  = data.from[0] === myColor;  // draw_offered / draw_declined / draw_withdrawn
  const iMoved  = data.turn[0] !== myColor;  // move: the mover is the side NOT to move
  ```

  Note `from` means a *square* on `move` (and on `clientMessage.From`) but a *color* on the three
  draw messages, so handlers must stay keyed on `type`.
- Every draw event is broadcast (decision 7), so each handler first checks `fromMe`:
  - `draw_offered` from the opponent opens the modal; from yourself it writes "Empate oferecido"
    into `statusEl`, so the offerer gets positive feedback rather than only a greyed-out button.
  - `draw_declined` reports "Adversário recusou o empate" only to the offerer, not to the decliner.
  - `draw_offered` also calls `hidePromoModal()`. Both overlays sit at `z-index: 999`, so an offer
    arriving mid-promotion would otherwise stack on top of it and leave `#promoModal` open with
    `pendingPromotion` set over a Game that may now be finished.
- The prompt — `#drawModal`, "Adversário propõe empate", **Aceitar** / **Recusar** sending
  `draw_response` with `accept` true/false — is a `.draw-offer` card pinned beside the board
  (decision 16), reusing only `.promo-box` for its surface. It must **not** reuse `.promo-overlay`
  (full-screen, blurred) nor `.promo-btn` for its buttons: that selector is a global listener wired
  to the promotion flow (`indexScript.js:52`) and carries a 2.5rem glyph size
  (`indexStyle.css:117-127`). Use a new `.draw-btn` class with its own rule in `indexStyle.css`.
  Sitting below the promotion overlay's `z-index`, it needs no interaction with `hidePromoModal`.
- The modal is **opened** by a `draw_offered` from the opponent, and by a `joined` whose
  `draw_offer` is the opponent's color — the reconnect case decision 6 exists for. This is not
  theoretical: an offer made before the opponent has connected reaches nobody, because `broadcast`
  iterates `r.conns` (`controllers/gameController.go:307-311`), so the `joined` replay is the only
  way that player ever learns of it.
- The modal is **closed** by Aceitar, Recusar, `draw_declined`, `draw_withdrawn`, `game_over`, and a
  `joined` whose `draw_offer` is null — so no ending or reconnect can leave a live Accept/Decline
  overlay over a dead socket.
- `METHOD_TEXT.agreement = 'acordo'` so the end screen reads "Empate por acordo".
- `ERROR_TEXT` gains `draw_already_offered` and `no_draw_offer`.

### Pre-existing bug fixed first (slice 0)

The `error` handler undoes the optimistic move for **any** code (`indexScript.js:275-285`). That is
only sound while every code is a verdict on the client's own move. The undo is gated to
`not_your_turn` and `illegal_move` — **not** `bad_message`, which is also produced for unparseable
or unknown-type messages (`gameController.go:225`, `:250`) and so can already roll back a legal,
server-accepted move. Without this, clicking Draw between a move and its echo desyncs the board
permanently. It is a bug the Draw button merely exposes, so it lands as its own commit before the
feature's UI slice rather than mixed into it.

## Behaviors (test cases)

### `game` package — `tests/game_test.go`

| # | Given | When | Then |
|---|---|---|---|
| G1 | Game in progress | White offers, Black accepts | `Result{Winner: NoColor, Method: Agreement}` from `RespondToDraw`, and from `State` |
| G2 | White has offered | Black declines | both calls return a nil Result and no error; `State.Result` nil; `State.DrawOffer == NoColor`; a legal move still succeeds |
| G3 | White has offered | White offers again without moving | `ErrDrawAlreadyOffered`; the first offer still stands |
| G4 | White offered and Black declined | White offers again without moving | `ErrDrawAlreadyOffered` |
| G5 | White offered and Black declined | White moves, then offers | offer succeeds; `State.DrawOffer == White` |
| G6 | White has offered | White moves | `Moved.DrawWithdrawn == true`; `State.DrawOffer == NoColor`; Black's subsequent accept returns `ErrNoDrawOffer` |
| G7 | White has offered | Black moves | `Moved.DrawWithdrawn == false`; `State.DrawOffer == White`; Black can still accept afterwards |
| G8 | Three moves played (`e2e4`, `e7e5`, `g1f3`) so **Black** is to move with the Clocks running; White has offered; 10 s pass | Black offers | `OfferDraw` returns `Result{NoColor, Agreement}`; Black's Clock is charged 10 s and none is running |
| G9 | Black offered; White declined; White moved; White then offered — Black has not moved since their own offer, so Black's right is spent | Black offers | the Game draws by agreement: mutual accept beats the one-per-move guard, no `ErrDrawAlreadyOffered` |
| G10 | No offer open | Black responds (accept, and separately decline) | `ErrNoDrawOffer` |
| G11 | White has offered | White responds to their own offer | `ErrNoDrawOffer`; the offer still stands |
| G12 | Game already ended (White's flag fell) | White offers; Black responds | both return `ErrGameOver`; the timeout Result stands; `State.DrawOffer == NoColor` |
| G13 | No moves played yet | White offers, Black accepts | `Result{NoColor, Agreement}` — offers are legal before the Clocks run |
| G14 | White offered before either side moved | 30 s pass with no move | `Result{NoColor, Aborted}`, not agreement |
| G15 | Three moves played (`e2e4`, `e7e5`, `g1f3`) so **Black** is to move; White has offered; 20 s pass | Black accepts | Black's Clock is charged 20 s in the final `State.Clocks`, White's is untouched, and no Clock is running |
| G16 | — | `State` is read before any offer, with White's offer open, and after the Game ends with an offer still open | `DrawOffer` is `NoColor`, then `White`, then `NoColor` again (decision 11) |
| G17 | — | `CanOfferDraw` is read for White: at the start; after White offers; after Black declines; after White moves; after Black **resigns** | `true`, `false`, `false`, `true`, `false` |

G8 and G15 look like near-duplicates. They are deliberate: they are the two callers of `acceptDraw`
(`OfferDraw`'s mutual-accept branch and `RespondToDraw`), and dropping either leaves one accept path
free to skip charging the Clocks. G17 ends via Resign rather than letting a Clock or the Abort
window run out, so the assertion cannot be confused with decision 10's `settle` priority.

### Room — `tests/room_test.go`

| # | Given | When | Then |
|---|---|---|---|
| R1 | both joined | White sends `draw_offer` | both sockets receive `draw_offered` with `from == "white"` |
| R2 | White has offered | Black sends `draw_response` with `accept:false` — and, as a second sub-case on a fresh offer, with the `accept` field **omitted** (decision 12) | both receive `draw_declined` with `from == "black"` in both sub-cases; the Room is still live — White then plays `e2e4` and both sockets receive `move` |
| R3 | White has offered | Black sends `draw_response` with `accept:true` | both receive `game_over` with `winner: null`, `method: "agreement"`; both sockets are closed; the Room is deregistered, both players return to `idle`, and the stored Result is `winner null, method agreement` (`expectEnded`) |
| R4 | White has offered | White moves | both receive `move`, then `draw_withdrawn` with `from == "white"` |
| R5 | White has offered | White sends `draw_offer` again | only White receives `error` with code `draw_already_offered` |
| R6 | no offer open | Black sends `draw_response` | only Black receives `error` with code `no_draw_offer` |

R5 and R6 prove the "only" half the way `TestRoom_RejectionsGoOnlyToTheSender` does
(`tests/room_test.go:254-256`): there is no "assert nothing arrived" primitive, so the opponent's
*next* received message is made a real `move`, which fails if an error reached them first.
| R7 | White has offered | Black sends `draw_offer` | both receive `game_over` with `winner: null`, `method: "agreement"` |
| R8 | White has offered, then White dials a second socket for the same player | the new socket joins | its `joined` carries `draw_offer: "white"` and `can_offer: false`; Black's own `joined` at the start of the test carried `draw_offer: null` and `can_offer: true` |
| R9 | White has offered, then **Black** dials a second socket | the new socket joins | its `joined` carries `draw_offer: "white"` and `can_offer: true` — the recipient's replay, which is what lets a reconnecting opponent ever see the offer |

R8 and R9 cannot use the `joinBoth` helper (`tests/room_test.go:102-110`): it drains the `joined`
payloads and returns only the sockets. They dial with `e.dial` and read `joined` themselves.
Two consequences of `cmdJoin` replacing a seat's socket (`gameController.go:187-189`): all reads on
the first socket must finish before the replacement is dialled, since the old conn is closed; and
the *opponent* receives an extra `opponent_joined` (`:201`) that any later `expect` on that socket
must account for.

## Edge cases and errors

- **Ended Game wins.** `OfferDraw`, `RespondToDraw` and `CanOfferDraw` all call `settle(now)` first;
  the first two return `ErrGameOver` if the Game is already over, so a fallen flag or an expired
  Abort window beats a draw offer that arrives late (G12, G14). The Room sends no reply for
  `ErrGameOver`; the loop announces the real Result on its next pass, as it does today.
- **Responding to your own offer** is `ErrNoDrawOffer`, not an accept (G11). Only the side the offer
  stands *against* may respond.
- **An offer never ends the Game by itself** except via the mutual-accept path (G8, G9).
- **Rejected calls change nothing**: a failed offer leaves `drawOffer` and `canOffer` untouched
  (G3, G11).
- **An illegal or out-of-turn move does not withdraw an offer** — only a move that is actually
  played does, since the offer is cleared inside `Move`'s success path.
- **A finished Game reports no offer**: `State.DrawOffer` is `NoColor` once `Result` is set, and
  `CanOfferDraw` is false (decision 11, G12, G16, G17).
- **A `draw_response` with no `accept` field declines** (decision 12).
- **Disconnect** still ends the Game as a win for the opponent; an open offer is discarded with the
  Game.
- The Room must not broadcast `draw_offered` when `OfferDraw` returned a Result (mutual accept) —
  the only announcement in that case is `game_over`.
- **A move that both withdraws an offer and ends the Game** (the offerer plays mate) produces
  `move`, then `draw_withdrawn`, then `game_over`, in that order. `draw_withdrawn` is not
  suppressed: it is a plain consequence of the move, and the client closes the modal on either.

## Implementation order

Each slice is one red-green TDD cycle (or a small handful) and one commit.

0. **Gate the error undo** — the pre-existing `indexScript.js:275-285` bug, on its own commit so the
   history stays bisectable. No Go change, verified by hand.
1. **Offer and accept** — `Agreement`, `ErrNoDrawOffer`, `drawOffer` field, `OfferDraw`,
   `RespondToDraw`, the `settle`/`ErrGameOver` guard every other command already opens with, the
   shared `acceptDraw` helper, `State.DrawOffer`. Covers G1, G2, G10, G11, G12, G16.
2. **One offer per move** — `canOffer`, `ErrDrawAlreadyOffered`, `CanOfferDraw`. This slice touches
   `Move` only to restore the mover's right; slice 3 adds the offer clear and `DrawWithdrawn`.
   Covers G3, G4, G5, G17.
3. **Withdrawal on the offerer's move** — `Moved.DrawWithdrawn`, clearing the mover's own offer
   inside `Move`. Covers G6, G7.
4. **Mutual offer accepts** — the opponent-offer-open branch at the top of `OfferDraw`, routed
   through `acceptDraw`. Covers G8, G9.
5. **Clocks and the Abort window** — offers before the Clocks run, the window expiring over an open
   offer, and the charge on accept. Expected to be **test-only**: slice 1's `acceptDraw` already
   charges. If a test here goes green on the first run, that is the plan working, not a missed step.
   Covers G13, G14, G15.
6. **Room: offer, decline and errors** — `draw_offer`/`draw_response` in `handleMessage`,
   `Accept` on `clientMessage`, the broadcast predicates, the two error codes. Covers R1, R2, R5, R6.
7. **Room: accept, withdrawal and replay** — `draw_withdrawn` relay, `draw_offer` and `can_offer` in
   `joined`. The accepted-Game-ends-and-persists half (R3, R7) needs no new code — the loop at
   `controllers/gameController.go:176-179` already finishes on a non-nil Result — so those two are
   regression tests over slice 6's wiring. Covers R3, R4, R7, R8, R9.
8. **UI** — Draw button and `updateDrawBtn()`, the draw modal with its own `.draw-btn` class and no
   outside dismiss, the three events keyed on `from`, `joined` replay in both directions,
   `METHOD_TEXT` and `ERROR_TEXT` entries. Verified by hand.
9. **Docs** — `GLOSSARY.md` gains **Draw offer** (with an `_Avoid_:` line like every other entry —
   draw request, draw proposal, and *agreement*, which is the Method string rather than the offer)
   and `agreement` in the Result entry; `README.md`
   gains the feature line **and** the protocol block updates: the two client messages, the three
   server messages, `draw_offer`/`can_offer` in the `joined` example, the two new codes in the
   `error` union, and `agreement` in the method list.

## Affected files

| File | Change |
|---|---|
| `game/game.go` | `Agreement`, two errors, `drawOffer`/`canOffer` state, `OfferDraw`, `RespondToDraw`, `CanOfferDraw`, `acceptDraw`, `Moved.DrawWithdrawn`, `State.DrawOffer`, clearing inside `Move` |
| `controllers/gameController.go` | `draw_offer`/`draw_response` handling, `clientMessage.Accept`, three broadcasts, two error codes, `joinedMessage.DrawOffer` and `.CanOffer` |
| `tests/game_test.go` | G1–G17 |
| `tests/room_test.go` | R1–R9 |
| `views/index.tmpl` | `grid-cols-3`, `drawBtn`, `#drawModal` with `.draw-btn` buttons |
| `public/scripts/indexScript.js` | button handler, `updateDrawBtn()`, modal open/close, event handling keyed on `from`, `joined` replay, gated error undo (slice 0), label maps |
| `public/assets/indexStyle.css` | `.draw-btn` rule for the modal's Aceitar/Recusar buttons |
| `GLOSSARY.md` | **Draw offer** term, `agreement` in the Result entry |
| `README.md` | feature line and the WebSocket Message Protocol block |
