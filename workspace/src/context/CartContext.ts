import { createContext, useContext, useState } from 'react';
import { Product } from '../types';

interface CartItem {
  product: Product;
  quantity: number;
}

interface CartContext {
  cartItems: CartItem[];
  addToCart: (product: Product) => void;
  removeFromCart: (product: Product) => void;
  updateQuantity: (product: Product, quantity: number) => void;
  clearCart: () => void;
  cartTotal: () => number;
  cartCount: () => number;
}

const CartContext = createContext<CartContext | undefined>(undefined);

const CartProvider = ({ children }) => {
  const [cartItems, setCartItems] = useState<CartItem[]>([]);
  const [total, setTotal] = useState(0);

  const addToCart = (product: Product) => {
    setCartItems([...cartItems, { product, quantity: 1 }]);
  };

  const removeFromCart = (product: Product) => {
    setCartItems(cartItems.filter((item) => item.product.id !== product.id));
  };

  const updateQuantity = (product: Product, quantity: number) => {
    setCartItems(
      cartItems.map((item) =>
        item.product.id === product.id ? { ...item, quantity } : item
      )
    );
  };

  const clearCart = () => {
    setCartItems([]);
  };

  const cartTotal = () => {
    return cartItems.reduce((acc, item) => acc + item.product.price * item.quantity, 0);
  };

  const cartCount = () => {
    return cartItems.length;
  };

  return (
    <CartContext.Provider
      value={{
        cartItems,
        addToCart,
        removeFromCart,
        updateQuantity,
        clearCart,
        cartTotal,
        cartCount,
      }}
    >
      {children}
    </CartContext.Provider>
  );
};

export { CartProvider, CartContext };