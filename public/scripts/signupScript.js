
    const form           = document.getElementById('signupForm');
    const btn            = document.getElementById('btnSubmit');
    const errorMsg       = document.getElementById('errorMsg');
    const successMsg     = document.getElementById('successMsg');
    const passwordInput  = document.getElementById('password');
    const confirmInput   = document.getElementById('confirmPassword');
    const usernameInput  = document.getElementById('username');
    const emailInput     = document.getElementById('email');

    // ── Password strength ──
    passwordInput.addEventListener('input', () => {
      const val = passwordInput.value;
      const strength = getStrength(val);
      const bars = [document.getElementById('s1'), document.getElementById('s2'),
                    document.getElementById('s3'), document.getElementById('s4')];
      const classes = ['', 'weak', 'medium', 'medium', 'strong'];
      const labels  = ['', 'Muito fraca', 'Fraca', 'Média', 'Forte'];
      const hintClasses = ['', 'error', 'error', '', 'success'];

      bars.forEach((b, i) => {
        b.className = i < strength ? classes[strength] : '';
      });

      const hint = document.getElementById('passwordHint');
      hint.textContent = val.length > 0 ? labels[strength] : '';
      hint.className = 'field-hint ' + (hintClasses[strength] || '');
    });

    function getStrength(p) {
      let score = 0;
      if (p.length >= 8) score++;
      if (/[A-Z]/.test(p)) score++;
      if (/[0-9]/.test(p)) score++;
      if (/[^A-Za-z0-9]/.test(p)) score++;
      return score;
    }

    // ── Confirm password ──
    confirmInput.addEventListener('input', checkConfirm);
    function checkConfirm() {
      const hint = document.getElementById('confirmHint');
      if (!confirmInput.value) { hint.textContent = ''; confirmInput.className = ''; return; }
      if (confirmInput.value === passwordInput.value) {
        hint.textContent = 'Senhas coincidem ✓';
        hint.className = 'field-hint success';
        confirmInput.classList.remove('invalid');
        confirmInput.classList.add('valid');
      } else {
        hint.textContent = 'Senhas não coincidem';
        hint.className = 'field-hint error';
        confirmInput.classList.remove('valid');
        confirmInput.classList.add('invalid');
      }
    }

    // ── Username validation ──
    usernameInput.addEventListener('input', () => {
      const val = usernameInput.value.trim();
      const hint = document.getElementById('usernameHint');
      if (!val) { hint.textContent = ''; usernameInput.className = ''; return; }
      if (val.length < 3) {
        hint.textContent = 'Mínimo 3 caracteres';
        hint.className = 'field-hint error';
        usernameInput.classList.add('invalid');
        usernameInput.classList.remove('valid');
      } else if (!/^[a-zA-Z0-9_]+$/.test(val)) {
        hint.textContent = 'Apenas letras, números e _';
        hint.className = 'field-hint error';
        usernameInput.classList.add('invalid');
        usernameInput.classList.remove('valid');
      } else {
        hint.textContent = 'Nome disponível ✓';
        hint.className = 'field-hint success';
        usernameInput.classList.remove('invalid');
        usernameInput.classList.add('valid');
      }
    });

    // ── Submit ──
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      errorMsg.style.display = 'none';
      successMsg.style.display = 'none';

      const username = usernameInput.value.trim();
      const email    = emailInput.value.trim();
      const password = passwordInput.value;
      const confirm  = confirmInput.value;

      if (!username || !email || !password || !confirm) {
        showError('Preencha todos os campos'); return;
      }
      if (username.length < 3 || !/^[a-zA-Z0-9_]+$/.test(username)) {
        showError('Nome de usuário inválido'); return;
      }
      if (password !== confirm) {
        showError('As senhas não coincidem'); return;
      }
      if (getStrength(password) < 2) {
        showError('Escolha uma senha mais forte'); return;
      }

      btn.disabled = true;
      btn.innerHTML = '<span class="spinner"></span>Criando conta...';

      try {
        const response = await fetch('/signup', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ username, email, password })
        });

        const data = await response.json().catch(() => ({}));

        if (!response.ok) {
          throw new Error(data.error || 'Erro ao criar conta');
        }

        showSuccess('Conta criada com sucesso! Redirecionando...');
        setTimeout(() => window.location.href = '/', 1500);

      } catch (error) {
        showError(error.message || 'Erro de conexão. Tente novamente.');
      } finally {
        btn.disabled = false;
        btn.innerHTML = 'Criar Conta';
      }
    });

    function showError(msg) {
      errorMsg.textContent = msg;
      errorMsg.style.display = 'block';
    }
    function showSuccess(msg) {
      successMsg.textContent = msg;
      successMsg.style.display = 'block';
    }
