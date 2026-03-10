import { createReducer } from 'immer';

const initialState = {
  cart: [],
};

const cartReducer = () => {
  const reducer = createReducer(initialState, {
    ADD_TO_CART: (state, { item }) => {
      state.cart = [...state.cart, item];
    },
    REMOVE_FROM_CART: (state, { item }) => {
      state.cart = state.cart.filter((cartItem) => cartItem !== item);
    },
    UPDATE_QUANTITY: (state, { item, quantity }) => {
      state.cart = state.cart.map((cartItem) => (cartItem === item ? { ...cartItem, quantity } : cartItem));
    },
    CLEAR_CART: (state) => {
      state.cart = [];
    },
  });

  return reducer;
};

export default cartReducer;