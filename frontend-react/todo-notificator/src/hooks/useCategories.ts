import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/api";

interface Category {
  id: number;
  name: string;
  color?: string;
}

interface CategoriesResponse {
  status: string;
  categories: Category[];
}

// ХУК ТОЛЬКО НА ЧТЕНИЕ
export const useCategories = () => {
  return useQuery({
    queryKey: ["categories"],
    queryFn: async () => {
      const { data } = await api.get<CategoriesResponse>("/categories");
      return data.categories;
    },
  });
};

// ХУК НА СОЗДАНИЕ
export const useCreateCategory = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (newCategory: { name: string; color?: string }) => {
      const { data } = await api.post<Category>("/categories", newCategory);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["categories"] });
    },
  });
};

// ХУК НА УДАЛЕНИЕ
export const useDeleteCategory = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/categories/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["categories"] });
    },
  });
};
