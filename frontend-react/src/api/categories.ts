import { api } from './client';

export interface Category {
  id: number;
  name: string;
}

export const getCategories = async (): Promise<Category[]> => {
  const { data } = await api.get('/categories');
  return data.categories || [];
};

export const createCategory = async (name: string): Promise<number> => {
  const { data } = await api.post('/categories', { name });
  return data.category_id;
};

export const updateCategory = async (id: number, name: string): Promise<void> => {
  await api.patch('/categories/' + id, { name });
};

export const deleteCategory = async (id: number): Promise<void> => {
  await api.delete('/categories/' + id);
};
