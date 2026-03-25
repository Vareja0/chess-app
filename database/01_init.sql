CREATE USER chess_user WITH PASSWORD 'chess_password';
ALTER DATABASE chess OWNER TO chess_user;
GRANT ALL PRIVILEGES ON DATABASE chess TO chess_user;
