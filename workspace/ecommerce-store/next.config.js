module.exports = {
  target: 'serverless',
  webpack: (config) => {
    config.module.rules.push({
      test: /\.svg$/,
      use: 'react-svg-loader',
    });
    return config;
  },
};