// src/models/CartItem.ts
import { Product } from './Product';

export type CartItem = {
  product: Product;
  quantity: number;
};