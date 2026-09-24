// Package sqlitestore 是示例应用的 SQLite 适配器，不属于 Ginx 核心。
package sqlitestore

import (
	"Ginx/persist"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrIdentityConflict = errors.New("player identity cannot be changed")

type Store struct{ db *sql.DB }

var _ persist.PlayerStore = (*Store)(nil)

// Open 显式打开数据库并初始化示例表。一个实例使用一个连接，事务提前获取写锁。
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("database path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(abs)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	query := url.Values{"_pragma": {"busy_timeout(3000)", "synchronous(FULL)"}, "_txlock": {"immediate"}}
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS example_players (
		player_id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL UNIQUE,
		data BLOB NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

type scanner interface{ Scan(...any) error }

func readPlayer(row scanner) (*persist.Player, error) {
	var id, timestamp string
	player := new(persist.Player)
	err := row.Scan(&id, &player.AccountID, &player.Data, &timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, persist.ErrPlayerNotFound
	}
	if err != nil {
		return nil, err
	}
	player.PlayerID, err = strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil, err
	}
	player.UpdatedAt, err = time.Parse(time.RFC3339Nano, timestamp)
	return player, err
}

const selectPlayer = `SELECT player_id, account_id, data, updated_at FROM example_players WHERE player_id = ?`

func (s *Store) Load(ctx context.Context, playerID uint64) (*persist.Player, error) {
	return readPlayer(s.db.QueryRowContext(ctx, selectPlayer, strconv.FormatUint(playerID, 10)))
}

func (s *Store) Save(ctx context.Context, player *persist.Player) error {
	if player == nil || player.PlayerID == 0 || player.AccountID == "" {
		return errors.New("invalid player")
	}
	data := player.Data
	if data == nil {
		data = []byte{}
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO example_players(player_id, account_id, data, updated_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(player_id) DO UPDATE SET data=excluded.data, updated_at=excluded.updated_at
		WHERE example_players.account_id=excluded.account_id`, strconv.FormatUint(player.PlayerID, 10), player.AccountID, data, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return ErrIdentityConflict
	}
	return err
}

func (s *Store) Delete(ctx context.Context, playerID uint64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM example_players WHERE player_id=?`, strconv.FormatUint(playerID, 10))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return persist.ErrPlayerNotFound
	}
	return err
}

// Update 的读取、回调修改和保存属于同一事务；回调不得重入 Store。
// 业务错误、SQL 错误或取消都回滚；提交成功才返回 nil。
func (s *Store) Update(ctx context.Context, playerID uint64, change func(*persist.Player) error) error {
	if change == nil {
		return errors.New("change callback is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id := strconv.FormatUint(playerID, 10)
	player, err := readPlayer(tx.QueryRowContext(ctx, selectPlayer, id))
	if err != nil {
		return err
	}
	accountID := player.AccountID
	if err := change(player); err != nil {
		return err
	}
	if player.PlayerID != playerID || player.AccountID != accountID {
		return ErrIdentityConflict
	}
	data := player.Data
	if data == nil {
		data = []byte{}
	}
	_, err = tx.ExecContext(ctx, `UPDATE example_players SET data=?, updated_at=? WHERE player_id=?`, data, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
