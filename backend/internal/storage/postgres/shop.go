package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"strings"
	"unicode/utf8"
)

func newWallet() domain.Wallet {
	return domain.Wallet{Version: 2, Coins: 50, CustomRewards: []domain.PersonalReward{}, Boosters: map[string]int64{}, OwnedItems: []string{"default"}, ActiveTheme: "default", ActiveSound: "default"}
}
func readWallet(ctx context.Context, db sqlExecutor, user int64) (domain.Wallet, error) {
	wallet := newWallet()
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM reward_wallets WHERE user_id=$1`, user).Scan(&raw)
	if err == sql.ErrNoRows {
		return wallet, nil
	}
	if err != nil {
		return wallet, err
	}
	err = json.Unmarshal(raw, &wallet)
	return wallet, err
}
func writeWallet(ctx context.Context, db sqlExecutor, user int64, wallet domain.Wallet) error {
	raw, err := json.Marshal(wallet)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO reward_wallets(user_id,data) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET data=excluded.data`, user, string(raw))
	return err
}
func shopError(code, message string, status int) error {
	return &domain.ContractError{Code: code, Message: message, Status: status}
}
func owns(wallet domain.Wallet, item string) bool {
	for _, id := range wallet.OwnedItems {
		if id == item {
			return true
		}
	}
	return false
}

func (s *Storage) GetShop(ctx context.Context, user int64) (domain.Wallet, []domain.LedgerEntry, error) {
	wallet, err := readWallet(ctx, s.executor(ctx), user)
	if err != nil {
		return wallet, nil, err
	}
	rows, err := s.executor(ctx).QueryContext(ctx, `SELECT id,coins,description,created_at FROM reward_ledger WHERE user_id=$1 ORDER BY id DESC LIMIT 30`, user)
	if err != nil {
		return wallet, nil, err
	}
	defer rows.Close()
	entries := []domain.LedgerEntry{}
	for rows.Next() {
		var e domain.LedgerEntry
		if err := rows.Scan(&e.ID, &e.Coins, &e.Description, &e.CreatedAt); err != nil {
			return wallet, nil, err
		}
		entries = append(entries, e)
	}
	return wallet, entries, rows.Err()
}

func (s *Storage) ShopAction(ctx context.Context, user int64, request domain.ShopAction) (domain.Wallet, error) {
	var result domain.Wallet
	err := s.withUserTx(ctx, user, func(tx *sql.Tx) error {
		wallet, err := readWallet(ctx, tx, user)
		if err != nil {
			return err
		}
		spend := int64(0)
		description := ""
		switch request.Action {
		case "import_legacy":
			if wallet.Imported {
				result = wallet
				return nil
			}
			importedItems := 0
			if request.Legacy != nil {
				// Browser coin/booster counts are untrusted and never credited.
				ids := map[string]bool{}
				for _, r := range request.Legacy.CustomRewards {
					if len(wallet.CustomRewards) >= 100 {
						break
					}
					r.Title = strings.TrimSpace(r.Title)
					if r.ID != "" && !ids[r.ID] && utf8.RuneCountInString(r.Title) > 0 && utf8.RuneCountInString(r.Title) <= 120 && r.Price > 0 && r.Price <= 1000000 {
						ids[r.ID] = true
						r.ID = uuid.NewString()
						wallet.CustomRewards = append(wallet.CustomRewards, r)
						importedItems++
					}
				}
				if request.Legacy.ActiveTheme == "ocean" || request.Legacy.ActiveTheme == "violet" {
					wallet.ActiveTheme = request.Legacy.ActiveTheme
					importedItems++
					if !owns(wallet, wallet.ActiveTheme) {
						wallet.OwnedItems = append(wallet.OwnedItems, wallet.ActiveTheme)
					}
				}
				if request.Legacy.ActiveSound == "bell" {
					wallet.ActiveSound = "bell"
					importedItems++
					if !owns(wallet, "bell") {
						wallet.OwnedItems = append(wallet.OwnedItems, "bell")
					}
				}
				// Unimplemented old booster inventory remains archived locally.
			}
			if importedItems == 0 {
				// An empty device must not reserve the one-time import from a device
				// that still has the user's legacy rewards.
				result = wallet
				return nil
			}
			wallet.Imported = true
		case "create_personal":
			title := strings.TrimSpace(request.Title)
			if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 120 || request.Price < 10 || request.Price > 1000000 {
				return shopError("INVALID_REWARD", "Укажи название до 120 символов и цену от 10 до 1000000 монет", 400)
			}
			if len(wallet.CustomRewards) >= 100 {
				return shopError("REWARD_LIMIT", "Можно сохранить до 100 личных наград", 400)
			}
			wallet.CustomRewards = append(wallet.CustomRewards, domain.PersonalReward{ID: uuid.NewString(), Title: title, Price: request.Price})
		case "delete_personal", "redeem_personal":
			index := -1
			for i, r := range wallet.CustomRewards {
				if r.ID == request.ItemID {
					index = i
					break
				}
			}
			if index < 0 {
				return shopError("REWARD_NOT_FOUND", "Личная награда не найдена", 404)
			}
			if request.Action == "delete_personal" {
				wallet.CustomRewards = append(wallet.CustomRewards[:index], wallet.CustomRewards[index+1:]...)
			} else {
				spend = wallet.CustomRewards[index].Price
				description = "Личная награда: " + wallet.CustomRewards[index].Title
			}
		case "buy":
			var item *domain.ShopItem
			for i := range domain.ShopCatalog {
				if domain.ShopCatalog[i].ID == request.ItemID {
					item = &domain.ShopCatalog[i]
					break
				}
			}
			if item == nil {
				return shopError("ITEM_NOT_FOUND", "Предмет не найден в магазине", 404)
			}
			if item.Kind != "booster" && owns(wallet, item.ID) {
				return shopError("ITEM_OWNED", "Этот предмет уже куплен", 409)
			}
			spend = item.Price
			description = "Покупка: " + item.Title
			if item.Kind == "booster" {
				wallet.Boosters[item.ID]++
			} else {
				wallet.OwnedItems = append(wallet.OwnedItems, item.ID)
			}
		case "equip":
			if request.ItemID != "default_sound" && !owns(wallet, request.ItemID) {
				return shopError("ITEM_NOT_OWNED", "Сначала приобрети этот предмет", 403)
			}
			switch request.ItemID {
			case "default", "ocean", "violet":
				wallet.ActiveTheme = request.ItemID
			case "bell":
				wallet.ActiveSound = "bell"
			case "default_sound":
				wallet.ActiveSound = "default"
			default:
				return shopError("ITEM_NOT_FOUND", "Предмет нельзя применить", 400)
			}
		case "activate_booster":
			if wallet.ActiveBooster {
				return shopError("BOOSTER_ACTIVE", "Бустер уже активен и ждёт засчитанную сессию", 409)
			}
			if wallet.Boosters["xp_boost"] < 1 {
				return shopError("BOOSTER_EMPTY", "Сначала купи бустер двойного фокуса", 409)
			}
			wallet.Boosters["xp_boost"]--
			wallet.ActiveBooster = true
		default:
			return shopError("INVALID_SHOP_ACTION", "Неизвестное действие магазина", 400)
		}
		if spend > wallet.Coins {
			return shopError("INSUFFICIENT_COINS", "Не хватает монет. Завершай задачи и сессии фокуса, чтобы накопить", 409)
		}
		wallet.Coins -= spend
		if spend > 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO reward_ledger(user_id,event_key,coins,description) VALUES($1,$2,$3,$4)`, user, "purchase:"+uuid.NewString(), -spend, description); err != nil {
				return err
			}
		}
		if err := writeWallet(ctx, tx, user, wallet); err != nil {
			return err
		}
		result = wallet
		return nil
	})
	return result, err
}

// Awards share the business transaction and deduplicate by the confirmed event.
func awardCoinsTx(ctx context.Context, tx *sql.Tx, user int64, event string, coins int64) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO reward_ledger(user_id,event_key,coins,description) VALUES($1,$2,$3,$4) ON CONFLICT(user_id,event_key) DO NOTHING`, user, event, coins, "Награда за подтверждённое действие")
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	wallet, err := readWallet(ctx, tx, user)
	if err != nil {
		return err
	}
	wallet.Coins += coins
	return writeWallet(ctx, tx, user, wallet)
}
func consumeFocusBoosterTx(ctx context.Context, tx *sql.Tx, user int64) (int, error) {
	wallet, err := readWallet(ctx, tx, user)
	if err != nil {
		return 0, err
	}
	if !wallet.ActiveBooster {
		return 0, nil
	}
	wallet.ActiveBooster = false
	if err := writeWallet(ctx, tx, user, wallet); err != nil {
		return 0, err
	}
	return domain.PomodoroRewardPoints, nil
}
func taskCoinEvent(id int64) string { return fmt.Sprintf("task:%d", id) }
