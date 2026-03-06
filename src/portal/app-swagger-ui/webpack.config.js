const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const { CleanWebpackPlugin } = require('clean-webpack-plugin');
const CopyPlugin = require('copy-webpack-plugin');

const defaultOutputPath = path.resolve(__dirname, 'dist');

module.exports = (_, argv = {}) => {
    const isDev = argv.mode !== 'production';
    const skipMinify = process.env.OPENAPI_UI_SKIP_MINIFY === 'true';
    const outputPath = process.env.OPENAPI_UI_OUTPUT_DIR
        ? path.resolve(process.env.OPENAPI_UI_OUTPUT_DIR)
        : defaultOutputPath;
    const htmlFilename =
        process.env.OPENAPI_UI_HTML_FILENAME ||
        (isDev ? 'index.html' : 'swagger-ui-index.html');
    const plugins = [
        new CleanWebpackPlugin(),
        new HtmlWebpackPlugin({
            template: 'index.html',
            filename: htmlFilename,
        }),
    ];

    if (isDev) {
        plugins.unshift(
            new CopyPlugin({
                patterns: ['favicon.ico'],
            })
        );
    }

    return {
        mode: isDev ? 'development' : 'production',
        entry: {
            app: require.resolve('./src/index'),
        },
        module: {
            rules: [
                {
                    test: /\.css$/,
                    use: [
                        { loader: 'style-loader' },
                        { loader: 'css-loader' },
                    ],
                },
            ],
        },
        plugins,
        output: {
            filename: 'swagger-ui.bundle.js',
            path: outputPath,
        },
        optimization: skipMinify
            ? {
                  minimize: false,
              }
            : undefined,
        devServer: isDev
            ? {
                  host: '0.0.0.0',
                  allowedHosts: 'all',
                  static: {
                      directory: path.resolve(__dirname),
                      watch: false,
                  },
                  hot: false,
                  liveReload: false,
                  client: false,
                  historyApiFallback: {
                      rewrites: [
                          {
                              from: /^\/devcenter-api-2\.0\/?$/,
                              to: '/index.html',
                          },
                      ],
                  },
                  proxy: undefined,
              }
            : undefined,
    };
};
