// =============================================
// Configurações
// =============================================
const WS_URL = (window.location.protocol === 'https:' ? 'wss://' : 'ws://') + window.location.host + '/ws/';
let ws = null;
let board = null;
let game = new Chess();
let myColor = null;
let playerRole = 'Espectador';

const statusEl       = document.getElementById('status');
const statusDot      = document.getElementById('statusDot');
const fenEl          = document.getElementById('fen');
const pgnEl          = document.getElementById('pgn');
const joinBtn        = document.getElementById('joinBtn');
const abortBtn       = document.getElementById('abortBtn');
const drawBtn        = document.getElementById('drawBtn');
const resignBtn      = document.getElementById('resignBtn');
const roomInput      = document.getElementById('roomInput');
const turnDot        = document.getElementById('turnDot');
const turnText       = document.getElementById('turnText');
const playerCard     = document.getElementById('playerCard');
const pieceIcon      = document.getElementById('pieceIcon');
const playerName     = document.getElementById('playerName');
const playerRoleEl   = document.getElementById('playerRoleEl');
const matchmakingBtn = document.getElementById('matchmakingBtn');
const mmSpinner      = document.getElementById('mmSpinner');
const mmBtnText      = document.getElementById('mmBtnText');
const promoModal     = document.getElementById('promoModal');
const drawOffer        = document.getElementById('drawOffer');
const drawOfferTitle   = document.getElementById('drawOfferTitle');
const drawOfferActions = document.getElementById('drawOfferActions');
const drawAcceptBtn    = document.getElementById('drawAcceptBtn');
const drawDeclineBtn   = document.getElementById('drawDeclineBtn');

// =============================================
// Promoção
// =============================================
let pendingPromotion = null;

function isPromotion(source, target) {
  const piece = game.get(source);
  if (!piece || piece.type !== 'p') return false;
  const rank = target[1];
  return (piece.color === 'w' && rank === '8') || (piece.color === 'b' && rank === '1');
}

function showPromoModal(source, target) {
  pendingPromotion = { source, target };
  promoModal.classList.add('active');
}

function hidePromoModal() {
  promoModal.classList.remove('active');
  pendingPromotion = null;
}

document.querySelectorAll('.promo-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    if (!pendingPromotion) return;

    const { source, target } = pendingPromotion;
    const promoPiece = btn.dataset.piece;

    const move = game.move({ from: source, to: target, promotion: promoPiece });
    if (move) {
      board.position(game.fen());
      highlightMove(source, target);

      if (ws && ws.readyState === WebSocket.OPEN) {
        pendingMove = true;
        ws.send(JSON.stringify({ type: 'move', from: source, to: target, promotion: promoPiece }));
      }
      updateStatus();
    } else {
      // Lance ilegal — volta o tabuleiro
      board.position(game.fen());
    }

    hidePromoModal();
  });
});

// Fechar modal clicando fora (cancela a promoção)
promoModal.addEventListener('click', (e) => {
  if (e.target === promoModal) {
    board.position(game.fen()); // volta a posição
    hidePromoModal();
  }
});

// =============================================
// Refresh
// =============================================
async function fetchWithRefresh(url, options = {}) {
  options.credentials = 'include';
  let res = await fetch(url, options);

  if (res.status === 401) {
    const refreshRes = await fetch('/refresh', {
      method: 'POST',
      credentials: 'include',
    });

    if (refreshRes.ok) {
      res = await fetch(url, options);
    } else {
      window.location.href = '/login';
      return null;
    }
  }

  return res;
}

// =============================================
// Status e atualização
// =============================================
function updateStatus() {
  const turn = game.turn();
  let text = turn === 'w' ? 'Brancas jogam' : 'Pretas jogam';

  if (game.game_over()) {
    if (game.in_checkmate()) {
      text = `Xeque-mate! ${turn === 'w' ? 'Pretas' : 'Brancas'} venceram!`;
    } else if (game.in_draw()) {
      text = 'Empate!';
    }
  } else if (game.in_check()) {
    text = `⚠ Xeque! ${text}`;
  }

  if (myColor && !game.game_over()) {
    text += turn === myColor ? ' — Sua vez' : ' — Aguarde';
  }

  statusEl.textContent = `${playerRole} | ${text}`;
  fenEl.innerHTML = `<span>FEN</span> ${game.fen()}`;
  pgnEl.innerHTML = `<span>PGN</span> ${game.pgn() || '—'}`;

  turnDot.className = 'turn-dot ' + (turn === 'w' ? 'active-white' : 'active-black');
  turnText.textContent = turn === 'w' ? 'Vez das Brancas' : 'Vez das Pretas';
}

function setDotState(state) {
  statusDot.className = 'status-dot ' + state;
}

// =============================================
// Relógios
// =============================================
// The server sends a Clock snapshot with joined, move and game_over; between snapshots we count down locally.
let clockSnapshot = null; // { white_ms, black_ms, running, receivedAt }
let gameOver = false;
let pendingMove = false;  // our optimistic local move, not yet confirmed by the server

function formatTime(ms) {
  const seconds = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(seconds / 60).toString().padStart(2, '0');
  const s = (seconds % 60).toString().padStart(2, '0');
  return `${m}:${s}`;
}

function setClocks(clocks) {
  clockSnapshot = { ...clocks, receivedAt: performance.now() };
  renderClocks();
}

function renderClocks() {
  if (!clockSnapshot) return;
  const elapsed = performance.now() - clockSnapshot.receivedAt;
  const white = clockSnapshot.white_ms - (clockSnapshot.running === 'white' ? elapsed : 0);
  const black = clockSnapshot.black_ms - (clockSnapshot.running === 'black' ? elapsed : 0);
  document.getElementById('whiteTimer').textContent = formatTime(white);
  document.getElementById('blackTimer').textContent = formatTime(black);
}

setInterval(renderClocks, 200);

// Abort is only possible before both sides have made their first move, i.e. while no Clock is running.
function updateAbortBtn() {
  abortBtn.disabled = gameOver || !clockSnapshot || clockSnapshot.running !== null;
}

// =============================================
// Oferta de empate
// =============================================
// canOffer mirrors the server's rule: one offer per own move, so it is spent until we move again.
// While an offer stands against us the modal is the only way to answer it — clicking Draw there
// would accept the offer outright, which is not what a button labelled "Draw" should do.
let canOffer = false;
let offerAgainstMe = false;

function updateDrawBtn() {
  drawBtn.disabled = gameOver || !canOffer || offerAgainstMe
    || !ws || ws.readyState !== WebSocket.OPEN;
}

// The wire says "white"/"black"; myColor is 'w'/'b'.
function isMine(color) {
  return !!color && !!myColor && color[0] === myColor;
}

// The offer sits in the column above the match actions, never over the board: it stands until the
// offerer moves, so the recipient is free to study the position, or play on, before answering.
function showDrawPrompt() {
  offerAgainstMe = true;
  drawOfferTitle.textContent = 'Adversário propõe empate';
  drawOfferActions.classList.remove('hidden');
  drawOffer.classList.add('active');
  updateDrawBtn();
}

// The offerer sees the same card without the buttons: the status line lives in a hidden container,
// so this is their only confirmation that the offer went out.
function showOfferSent() {
  drawOfferTitle.textContent = 'Empate oferecido — aguardando resposta';
  drawOfferActions.classList.add('hidden');
  drawOffer.classList.add('active');
}

function hideDrawOffer() {
  offerAgainstMe = false;
  drawOffer.classList.remove('active');
  updateDrawBtn();
}

function respondToDraw(accept) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'draw_response', accept }));
  }
  hideDrawOffer();
}

// =============================================
// Resultado
// =============================================
const METHOD_TEXT = {
  checkmate: 'xeque-mate',
  stalemate: 'afogamento',
  insufficient_material: 'material insuficiente',
  threefold_repetition: 'repetição tripla',
  fifty_moves: 'regra dos 50 lances',
  resign: 'desistência',
  timeout: 'tempo',
  disconnect: 'desconexão',
  agreement: 'acordo',
};

function resultText(winner, method) {
  if (method === 'aborted') return 'Partida abortada';
  const how = METHOD_TEXT[method] || method;
  if (winner === 'white') return `Brancas venceram por ${how}`;
  if (winner === 'black') return `Pretas venceram por ${how}`;
  return `Empate por ${how}`;
}

const ERROR_TEXT = {
  not_your_turn: 'Não é sua vez',
  illegal_move: 'Movimento ilegal',
  cannot_abort: 'Não é possível abortar após os primeiros lances',
  bad_message: 'Mensagem inválida',
  draw_already_offered: 'Você já ofereceu empate neste lance',
  no_draw_offer: 'Nenhuma oferta de empate pendente',
};

// Codes that are a verdict on the move we just sent, and so may undo it locally.
const MOVE_ERRORS = new Set(['not_your_turn', 'illegal_move']);

// =============================================
// WebSocket
// =============================================
function connectWebSocket(roomCode) {
  const url = new URL(window.location.href);
  url.searchParams.set('room', roomCode);
  window.history.pushState({}, '', url);

  if (ws) ws.close();

  ws = new WebSocket(WS_URL + roomCode);

  ws.onopen = () => {
    setDotState('waiting');
    statusEl.textContent = `Sala ${roomCode} — aguardando adversário...`;
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      console.log(data);

      if (data.type === 'joined') {
        gameOver = false;
        pendingMove = false;
        game.reset();
        for (const uci of data.moves) {
          game.move({ from: uci.slice(0, 2), to: uci.slice(2, 4), promotion: uci.slice(4) || undefined });
        }
        board.position(game.fen());
        myColor = data.color === 'white' ? 'w' : 'b';
        playerRole = myColor === 'w' ? 'Brancas' : 'Pretas';
        setDotState(data.opponent_connected ? 'connected' : 'waiting');
        setClocks(data.clocks);
        updateAbortBtn();
        resignBtn.disabled = false;

        // Replay any offer that is still standing: ours keeps the button spent, the opponent's
        // reopens the prompt. An offer made before we connected arrives no other way.
        canOffer = data.can_offer;
        if (!data.draw_offer) {
          hideDrawOffer();
        } else if (isMine(data.draw_offer)) {
          hideDrawOffer(); // clears offerAgainstMe before the card is reused
          showOfferSent();
        } else {
          showDrawPrompt();
        }

        board.orientation(myColor === 'b' ? 'black' : 'white');

        playerCard.style.display = 'block';
        pieceIcon.className = 'piece-icon ' + (myColor === 'w' ? 'white' : 'black');
        pieceIcon.textContent = myColor === 'w' ? '♔' : '♚';
        playerName.textContent = playerRole;
        playerRoleEl.textContent = data.opponent_connected ? 'Adversário conectado' : 'Aguardando adversário';
        updateStatus();
      }

      if (data.type === 'opponent_joined') {
        setDotState('connected');
        statusEl.textContent = data.message || 'Adversário conectado!';
        if (playerRoleEl) playerRoleEl.textContent = 'Adversário conectado';
      }

      if (data.type === 'move') {
        const mine = pendingMove && game.turn() !== myColor;
        if (mine) {
          pendingMove = false; // already on our board
        } else {
          game.move({ from: data.from, to: data.to, promotion: data.promotion || undefined });
        }
        board.position(game.fen());
        highlightMove(data.from, data.to);
        setClocks(data.clocks);
        updateAbortBtn();
        // The mover is the side that is now NOT to move; moving earns back the right to offer.
        if (!isMine(data.turn)) {
          canOffer = true;
        }
        updateDrawBtn();
        updateStatus();
      }

      if (data.type === 'draw_offered') {
        if (isMine(data.from)) {
          canOffer = false;
          updateDrawBtn();
          showOfferSent();
        } else {
          showDrawPrompt();
        }
      }

      if (data.type === 'draw_declined') {
        hideDrawOffer();
        if (!isMine(data.from)) {
          statusEl.textContent = `${playerRole} | Adversário recusou o empate`;
        }
      }

      if (data.type === 'draw_withdrawn') {
        hideDrawOffer();
        if (!isMine(data.from)) {
          statusEl.textContent = `${playerRole} | Oferta de empate retirada`;
        }
      }

      if (data.type === 'error') {
        // Only a verdict on the move itself may drop the optimistic move; any other rejection
        // (a draw command, an unknown message) would roll back a move the server has accepted.
        if (pendingMove && MOVE_ERRORS.has(data.code)) {
          game.undo();
          pendingMove = false;
        }
        board.position(game.fen());
        updateStatus();
        statusEl.textContent = `${playerRole} | ${ERROR_TEXT[data.code] || data.message}`;
        console.warn('Erro do servidor:', data.code, data.message);
      }

      if (data.type === 'game_over') {
        gameOver = true;
        setClocks(data.clocks);
        setDotState('');
        statusEl.textContent = `Fim de jogo: ${resultText(data.winner, data.method)}`;
        resignBtn.disabled = true;
        updateAbortBtn();
        hideDrawOffer();
      }

    } catch (e) {
      console.error('Erro ao parsear mensagem WS:', e);
    }
  };

  ws.onclose = () => {
    const url = new URL(window.location.href);
    url.searchParams.delete('room');
    window.history.pushState({}, '', url);

    resignBtn.disabled = true;
    abortBtn.disabled = true;
    drawBtn.disabled = true;
    if (gameOver) return; // keep the Result on screen

    setDotState('error');
    statusEl.textContent = 'Desconectado. Tente novamente.';
  };

  ws.onerror = (err) => {
    console.error('WebSocket error:', err);
    setDotState('error');
    statusEl.textContent = 'Erro na conexão WebSocket';
  };
}

// =============================================
// Chessboard
// =============================================
const config = {
  draggable: true,
  position: 'start',

  onDragStart: (source, piece) => {
    if (gameOver || game.game_over() || myColor === null) return false;
    if (game.turn() !== myColor) return false;
    if ((myColor === 'w' && piece.search(/^b/) !== -1) ||
        (myColor === 'b' && piece.search(/^w/) !== -1)) return false;
  },

  onDrop: (source, target) => {
    // Se for promoção, mostra o modal e não faz nada ainda
    if (isPromotion(source, target)) {
      // Testa se o lance é legal com qualquer promoção
      const testMove = game.move({ from: source, to: target, promotion: 'q' });
      if (testMove === null) return 'snapback'; // lance ilegal
      game.undo(); // desfaz o teste
      showPromoModal(source, target);
      return; // peça fica visualmente no target até o jogador escolher
    }

    // Lance normal (sem promoção)
    const move = game.move({ from: source, to: target });
    if (move === null) return 'snapback'; // lance ilegal — volta a peça

    highlightMove(source, target);

    if (ws && ws.readyState === WebSocket.OPEN) {
      pendingMove = true;
      ws.send(JSON.stringify({ type: 'move', from: source, to: target, promotion: '' }));
    }
    updateStatus();
  },

  onSnapEnd: () => {
    board.position(game.fen());
  },

  pieceTheme: 'https://chessboardjs.com/img/chesspieces/wikipedia/{piece}.png'
};

board = Chessboard('board', config);
updateStatus();

// Prevent page scroll while dragging a piece on touch devices
const boardEl = document.getElementById('board');
let draggingPiece = false;

boardEl.addEventListener('touchstart', () => { draggingPiece = true; }, { passive: true });
boardEl.addEventListener('touchend',   () => { draggingPiece = false; });
boardEl.addEventListener('touchmove',  (e) => { if (draggingPiece) e.preventDefault(); }, { passive: false });

// Conectar automaticamente se tiver room na URL
const params = new URLSearchParams(window.location.search);
const roomFromURL = params.get('room');
if (roomFromURL) {
  roomInput.value = roomFromURL;
  connectWebSocket(roomFromURL);
}

// =============================================
// Highlights
// =============================================
function clearHighlights() {
  $('#board .square-55d63').removeClass('highlight-from highlight-to');
}

function highlightMove(from, to) {
  clearHighlights();
  $('#board .square-' + from).addClass('highlight-from');
  $('#board .square-' + to).addClass('highlight-to');
}

// =============================================
// Login
// =============================================
document.getElementById('loginBtn').addEventListener('click', () => {
  window.location.href = '/login';
});

// =============================================
// Matchmaking
// =============================================
const MATCHMAKING_URL = '/matchmaking';
let isSearching = false;
let selectedTimeMode = 'rapid';

const timeModeButtons = document.querySelectorAll('.time-mode-btn');

function updateTimeModeUI() {
  timeModeButtons.forEach(btn => {
    const isSelected = btn.dataset.mode === selectedTimeMode;
    const titleEl = btn.querySelector('div:first-child');
    const subtitleEl = btn.querySelector('div:last-child');
    if (isSelected) {
      btn.classList.remove('border-white/5', 'bg-surface-container-high');
      btn.classList.add('border-primary/50', 'bg-primary-container');
      titleEl.className = 'text-xs font-bold text-on-primary-container';
      subtitleEl.className = 'text-[10px] text-primary';
      document.getElementById('whiteTimer').textContent = btn.dataset.time;
      document.getElementById('blackTimer').textContent = btn.dataset.time;
    } else {
      btn.classList.remove('border-primary/50', 'bg-primary-container');
      btn.classList.add('border-white/5', 'bg-surface-container-high');
      titleEl.className = 'text-xs font-bold text-on-surface';
      subtitleEl.className = 'text-[10px] text-on-surface-variant';
    }
  });
}

timeModeButtons.forEach(btn => {
  btn.addEventListener('click', () => {
    if (isSearching) return;
    selectedTimeMode = btn.dataset.mode;
    updateTimeModeUI();
  });
});

updateTimeModeUI();

function setMatchmakingSearching(searching) {
  isSearching = searching;
  matchmakingBtn.classList.toggle('searching', searching);
  mmSpinner.style.display = searching ? 'block' : 'none';
  mmBtnText.textContent = searching ? 'Procurando adversário...' : '⚔ Buscar Partida';
  timeModeButtons.forEach(btn => { btn.disabled = searching; });
}

matchmakingBtn.addEventListener('click', async () => {
  if (isSearching) {
    setMatchmakingSearching(false);
    setDotState('');
    statusEl.textContent = 'Busca cancelada.';
    await fetchWithRefresh('/matchmaking/cancel', { method: 'POST' });
    return;
  }

  setMatchmakingSearching(true);
  setDotState('waiting');
  statusEl.textContent = 'Buscando adversário...';

  try {
    const res = await fetchWithRefresh(MATCHMAKING_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ time_mode: selectedTimeMode }),
    });
    if (!res || !res.ok) throw new Error(`HTTP ${res ? res.status : 'null'}`);

    const data = await res.json();
    if (data.message === 'matchmaking cancelled') {
      setMatchmakingSearching(false);
      statusEl.textContent = data.message;
      return;
    }

    const roomCode = data.room;
    setMatchmakingSearching(false);
    statusEl.textContent = `Partida encontrada! Sala: ${roomCode}`;
    roomInput.value = roomCode;
    connectWebSocket(roomCode);
  } catch (err) {
    setMatchmakingSearching(false);
    setDotState('error');
    statusEl.textContent = 'Erro no matchmaking: ' + err.message;
    console.error('Matchmaking error:', err);
  }
});

// =============================================
// Botões
// =============================================
joinBtn.addEventListener('click', () => {
  const room = roomInput.value.trim();
  if (!room) { alert('Digite o código da sala!'); return; }
  connectWebSocket(room);
});

abortBtn.addEventListener('click', () => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'abort' }));
  }
});

drawBtn.addEventListener('click', () => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'draw_offer' }));
    // Spend the right here too: a double-click would otherwise earn a rejection from the server.
    canOffer = false;
    updateDrawBtn();
  }
});

drawAcceptBtn.addEventListener('click', () => respondToDraw(true));
drawDeclineBtn.addEventListener('click', () => respondToDraw(false));

resignBtn.addEventListener('click', () => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    if (confirm('Tem certeza que deseja desistir?')) {
      ws.send(JSON.stringify({ type: 'resign' }));
    }
  }
});
