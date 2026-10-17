import { useState } from "react";
import { Link } from "react-router-dom";
import { Folder, Pencil, Trash2, Plus } from "lucide-react";
import { useApp } from "../../app/context";
import { api, errorText, json } from "../../lib/api";
import type { Category } from "../../lib/types";
import { Button, Heading, Sheet, State } from "../../components/ui";
import s from "../../styles/App.module.css";

export function CategoriesPage() {
  const { data, refresh, notify } = useApp();
  const [editing, setEditing] = useState<Category | null>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState<Category | null>(null);
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(editing ? `/categories/${editing.id}` : "/categories", {
        method: editing ? "PATCH" : "POST",
        body: json({ name: name.trim() }),
      });
      setOpen(false);
      notify("Категория сохранена.");
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Heading
        eyebrow="ПОРЯДОК В ДЕТАЛЯХ"
        title="Категории"
        action={
          <Button
            onClick={() => {
              setEditing(null);
              setName("");
              setError("");
              setOpen(true);
            }}
          >
            <Plus size={17} />
            Добавить категорию
          </Button>
        }
      >
        Раздели дела по направлениям. Использовано {
          data.categories.length
        } из{" "}
        {data.user.categories_limit === -1 ? "∞" : data.user.categories_limit}.
      </Heading>
      <div className={s.cardGrid}>
        {data.categories.map((category) => (
          <section key={category.id} className={s.card}>
            <Folder className={s.green} />
            <h2>
              <Link to={`/app/tasks?category=${category.id}`}>
                {category.name}
              </Link>
            </h2>
            <div className={s.row}>
              <Button
                quiet
                onClick={() => {
                  setEditing(category);
                  setName(category.name);
                  setError("");
                  setOpen(true);
                }}
              >
                <Pencil size={15} />
                Изменить
              </Button>
              <button
                className={s.iconButton}
                onClick={() => {
                  setError("");
                  setDeleting(category);
                }}
                aria-label={`Удалить категорию ${category.name}`}
              >
                <Trash2 size={16} />
              </button>
            </div>
          </section>
        ))}
      </div>
      {!data.categories.length && (
        <State title="У каждого дела будет своё место">
          Создай первую категорию: например, «Личное» или «Работа».
        </State>
      )}
      <Sheet
        open={open}
        onClose={() => {
          if (!busy) setOpen(false);
        }}
        title={editing ? "Изменить категорию" : "Новая категория"}
        wide
      >
        <form className={s.stack} onSubmit={save}>
          <label className={s.field}>
            Название
            <input
              autoFocus
              required
              maxLength={255}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          <Button disabled={busy || !name.trim()}>Сохранить</Button>
        </form>
      </Sheet>
      <Sheet
        open={!!deleting}
        onClose={() => {
          if (!busy) setDeleting(null);
        }}
        title="Удалить категорию?"
        description="Задачи останутся, но потеряют эту категорию."
        wide
      >
        <div className={s.stack}>
          {error && (
            <p role="alert" className={s.error}>
              {error}
            </p>
          )}
          <Button
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await api(`/categories/${deleting!.id}`, { method: "DELETE" });
                setDeleting(null);
                await refresh();
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            Удалить категорию
          </Button>
        </div>
      </Sheet>
    </>
  );
}
