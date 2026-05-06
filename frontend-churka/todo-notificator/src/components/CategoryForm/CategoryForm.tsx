import React, { useState } from "react";
import { Button, message } from "antd";
import { useCreateCategory } from "../../hooks/useCategories";
import "./CategoryForm.scss";

const CategoryForm: React.FC = () => {
  const [name, setName] = useState("");

  const { mutate: createCategory, isPending } = useCreateCategory();

  const handleAdd = () => {
    if (!name.trim()) {
      message.warning("Введите название категории");
      return;
    }

    createCategory(
      { name },
      {
        onSuccess: () => {
          message.success(`Категория "${name}" создана`);
          setName(""); // Очищаем поле
        },
        onError: () => {
          message.error("Не удалось создать категорию");
        },
      },
    );
  };

  return (
    <div className="category-form-container">
      <h3 className="form-title">Новая категория</h3>
      <div className="form-row">
        <input
          className="custom-input"
          placeholder="Название категории"
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleAdd()}
        />

        <Button
          className="add-category-btn"
          type="primary"
          onClick={handleAdd}
          loading={isPending}
        >
          Создать
        </Button>
      </div>
    </div>
  );
};

export default CategoryForm;
