import { createContext, useState, useEffect } from 'react';
import cartReducer from './cartReducer';

const CartContext = createContext();

const CartProvider = ({ children }) => {
  const [cart, dispatch] = cartReducer();

  const addToCart = (item) => dispatch({ type: 'ADD_TO_CART', item });
  const removeFromCart = (item) => dispatch({ type: 'REMOVE_FROM_CART', item });
  const updateQuantity = (item, quantity) => dispatch({ type: 'UPDATE_QUANTITY', item, quantity });
  const clearCart = () => dispatch({ type: 'CLEAR_CART' });
  const cartTotal = () => cart.reduce((acc, item) => acc + item.price * item.quantity, 0);
  const cartCount = () => cart.length;

  useEffect(() => {
    // Initialize cart if necessary
  }, []);

  return (
    <CartContext.Provider value={{ cart, dispatch, addToCart, removeFromCart, updateQuantity, clearCart, cartTotal, cartCount }}>
      {children}
    </CartContext.Provider>
  );
};

export { CartProvider, CartContext };