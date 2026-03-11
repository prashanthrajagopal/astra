import Image from 'next/image';

const ProductImage = () => {
  const productId = process.env.NEXT_ID;

  return (
    <div className="flex justify-center">
      <Image
        src={`/images/products/${productId}.jpg`}
        alt={productId}
        width={400}
        height={400}
      />
    </div>
  );
};

export default ProductImage;