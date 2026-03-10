import { CartContext, CartStateProvider } from './state';

export const useCart = () => {
  const context = useContext(CartContext);

  if (!context) {
    throw new Error('useCart must be used within a CartStateProvider');
  }

  return context;
};