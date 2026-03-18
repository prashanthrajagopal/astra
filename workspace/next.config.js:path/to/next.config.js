const withTM = require("next-transpile-modules")(["@tailwindcss/ui"]);
module.exports = withTM({
  reactStrictMode: true,
});