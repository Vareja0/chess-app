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
const startBtn       = document.getElementById('startBtn');
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
// WebSocket
// =============================================
function formatTime(seconds) {
  const m = Math.floor(seconds / 60).toString().padStart(2, '0');
  const s = (seconds % 60).toString().padStart(2, '0');
  return `${m}:${s}`;
}

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

      if (data.type === 'timer') {
        document.getElementById('whiteTimer').textContent = formatTime(data.white_time);
        document.getElementById('blackTimer').textContent = formatTime(data.black_time);
      }

      if (data.type === 'joined') {
        game.load(data.fen);
        board.position(data.fen);
        myColor = data.color === 'w' ? 'w' : 'b';
        playerRole = myColor === 'w' ? 'Brancas' : 'Pretas';
        setDotState('connected');
        statusEl.textContent = `Você é ${playerRole} — ${data.message}`;
        startBtn.disabled = false;
        resignBtn.disabled = false;

        // Orientar o tabuleiro
        if (myColor === 'b') {
          board.orientation('black');
        } else {
          board.orientation('white');
        }

        playerCard.style.display = 'block';
        pieceIcon.className = 'piece-icon ' + (myColor === 'w' ? 'white' : 'black');
        pieceIcon.textContent = myColor === 'w' ? '♔' : '♚';
        playerName.textContent = playerRole;
        playerRoleEl.textContent = 'Conectado';
      }

      if (data.type === 'opponent_joined') {
        setDotState('connected');
        statusEl.textContent = data.message || 'Adversário conectado!';
        if (playerRoleEl) playerRoleEl.textContent = 'Adversário conectado';
      }

      if (data.type === 'start') {
        clearHighlights();
        game.reset();
        board.position('start');
        updateStatus();
      }

      if (data.type === 'move') {
        const move = game.move({
          from: data.from,
          to: data.to,
          promotion: data.promotion || undefined
        });
        if (move) {
          board.position(game.fen());
          highlightMove(data.from, data.to);
          updateStatus();
        }
      }

      if (data.type === 'error') {
        // Servidor rejeitou o lance — volta o tabuleiro para a posição válida
        board.position(game.fen());
        console.warn('Erro do servidor:', data.message);
      }

      if (data.type === 'game_over') {
        setDotState('');
        statusEl.textContent = `Fim de jogo: ${data.outcome} ${data.method ? '(' + data.method + ')' : ''}`;
        resignBtn.disabled = true;
      }

      if (data.type === 'opponent_disconnected') {
        setDotState('error');
        statusEl.textContent = 'Adversário desconectou. Jogo pausado.';
        startBtn.disabled = true;
      }

    } catch (e) {
      console.error('Erro ao parsear mensagem WS:', e);
    }
  };

  ws.onclose = () => {
    const url = new URL(window.location.href);
    url.searchParams.delete('room');
    window.history.pushState({}, '', url);

    setDotState('error');
    statusEl.textContent = 'Desconectado. Tente novamente.';
    startBtn.disabled = true;
    resignBtn.disabled = true;
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
    if (game.game_over() || myColor === null) return false;
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

function setMatchmakingSearching(searching) {
  isSearching = searching;
  matchmakingBtn.classList.toggle('searching', searching);
  mmSpinner.style.display = searching ? 'block' : 'none';
  mmBtnText.textContent = searching ? 'Procurando adversário...' : '⚔ Buscar Partida';
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
    const res = await fetchWithRefresh(MATCHMAKING_URL, { method: 'POST' });
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

startBtn.addEventListener('click', () => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'start' }));
  }
});

resignBtn.addEventListener('click', () => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    if (confirm('Tem certeza que deseja desistir?')) {
      ws.send(JSON.stringify({ type: 'resign' }));
    }
  }
});
