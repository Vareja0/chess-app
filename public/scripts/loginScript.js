
    const form = document.getElementById('loginForm');
    const btn = document.getElementById('btnSubmit');
    const errorMsg = document.getElementById('errorMsg');
    const successMsg = document.getElementById('successMsg');

    form.addEventListener('submit', async (e) => {
      e.preventDefault();

      // Limpa mensagens anteriores
      errorMsg.style.display = 'none';
      successMsg.style.display = 'none';

      const email = document.getElementById('email').value.trim();
      const password = document.getElementById('password').value;

      // Validação básica no frontend
      if (!email || !password) {
        showError('Preencha todos os campos');
        return;
      }

      console.log(JSON.stringify({ email, password }))

      // Desabilita botão e mostra loading
      btn.disabled = true;
      btn.textContent = 'Entrando...';
      form.classList.add('loading');

      try {
        const response = await fetch('/login', {
          method: 'POST',
          headers: {
            "Content-Type": "application/json", 
          },
          body: JSON.stringify({ email, password })
        });

        

        if (!response.ok) {
          throw new Error(data.error || 'Erro ao fazer login');
        }

        // Sucesso
        showSuccess('Login realizado com sucesso! Redirecionando...');
        setTimeout(() => window.location.href = '/', 1200);

      } catch (error) {
        showError(error.message || 'Erro de conexão. Tente novamente.');
        console.error('Erro no login:', error);
      } finally {
        // Restaura botão
        btn.disabled = false;
        btn.textContent = 'Entrar';
        form.classList.remove('loading');
      }
    });

    function showError(message) {
      errorMsg.textContent = message;
      errorMsg.style.display = 'block';
    }

    function showSuccess(message) {
      successMsg.textContent = message;
      successMsg.style.display = 'block';
    }

