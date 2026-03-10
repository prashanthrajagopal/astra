import { Product } from '../types';

const ProductCard = ({ product }: { product: Product }) => {
  return (
    <div className="bg-white p-4 shadow-md rounded">
      <img src={product.image} alt={product.name} className="w-full h-64 object-cover" />
      <h2 className="text-lg font-bold">{product.name}</h2>
      <p className="text-gray-500">{product.description}</p>
      <p className="text-indigo-500 font-bold">${product.price}</p>
      <button className="bg-indigo-500 p-2 rounded">Add to Cart</button>
    </div>
  );
};

export default ProductCard;