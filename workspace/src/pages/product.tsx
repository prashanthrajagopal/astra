import Head from 'next/head';
import { useRouter } from 'next/router';
import { useState, useEffect } from 'react';
import Image from 'next/image';
import { formatCurrency } from '../utils/format';
import { useProduct } from '../hooks/useProduct';
import { useCart } from '../hooks/useCart';
import { Button } from '@material-ui/core';
import { Typography } from '@material-ui/core';
import { Grid } from '@material-ui/core';
import { useQuantity } from '../hooks/useQuantity';

const Product = () => {
  const router = useRouter();
  const { productId } = router.query;
  const { product } = useProduct(productId);
  const { cart, updateCart } = useCart();
  const { quantity, setQuantity } = useQuantity(productId);

  useEffect(() => {
    if (!product) {
      router.push('/404');
    }
  }, [product, router]);

  const handleAddToCart = () => {
    updateCart({ productId, quantity: parseInt(quantity, 10) });
  };

  return (
    <div>
      <Head>
        <title>{product.name} - Product Detail</title>
      </Head>
      <Grid container spacing={4}>
        <Grid item xs={12} md={6}>
          <Image
            src={product.image}
            alt={product.name}
            width={500}
            height={500}
            layout="responsive"
          />
        </Grid>
        <Grid item xs={12} md={6}>
          <Typography variant="h4" component="h2">
            {product.name}
          </Typography>
          <Typography variant="body1" component="p">
            {product.description}
          </Typography>
          <Typography variant="h5" component="h3">
            {formatCurrency(product.price)}
          </Typography>
          <Typography variant="body1" component="p">
            Rating: {product.rating}
          </Typography>
          <Typography variant="body1" component="p">
            Stock Status: {product.inStock ? 'In Stock' : 'Out of Stock'}
          </Typography>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={6}>
              <Button
                variant="contained"
                color="primary"
                onClick={handleAddToCart}
              >
                Add to Cart
              </Button>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Typography variant="body1" component="p">
                Quantity: {quantity}
              </Typography>
              <Button
                variant="outlined"
                onClick={() => setQuantity(quantity - 1)}
              >
                -
              </Button>
              <Button
                variant="outlined"
                onClick={() => setQuantity(quantity + 1)}
              >
                +
              </Button>
            </Grid>
          </Grid>
        </Grid>
      </Grid>
    </div>
  );
};

export default Product;