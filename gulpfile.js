var gulp = require('gulp');
var csso = require('gulp-csso');
var concat = require('gulp-concat');
var uglify = require('gulp-uglify');
var sourcemaps = require("gulp-sourcemaps");
var googleWebFonts = require('gulp-google-webfonts')
var clean = require('gulp-clean');
var ifEnv = require('gulp-if-env');
var merge = require('merge-stream');

gulp.task('js', function(){
    return gulp.src([
        'node_modules/jquery/dist/jquery.js',

        'node_modules/katex/dist/katex.js',
        'node_modules/katex/dist/contrib/auto-render.js',

        'node_modules/codemirror/lib/codemirror.js',
        'node_modules/codemirror/addon/display/placeholder.js',
        'node_modules/codemirror/addon/edit/matchbrackets.js',
        'node_modules/codemirror/mode/clike/clike.js',
        'node_modules/codemirror/mode/python/python.js',
        'node_modules/codemirror/mode/pascal/pascal.js',
        'node_modules/codemirror/mode/javascript/javascript.js',

        'static/src/*.js'
    ])
    .pipe(ifEnv.not('production', sourcemaps.init()))
    .pipe(concat('obijudge.js'))
    .pipe(ifEnv.not('production', sourcemaps.write()))
    .pipe(ifEnv('production', uglify()))
    .pipe(gulp.dest('static/dist'));
})

gulp.task('css', function(){
    return gulp.src([
        'node_modules/normalize.css/normalize.css',
        'node_modules/skeleton-css/css/skeleton.css',
        'node_modules/codemirror/lib/codemirror.css',
        'node_modules/katex/dist/katex.css',
        'static/src/*.css'
    ])
    .pipe(ifEnv.not('production', sourcemaps.init()))
    .pipe(ifEnv('production', csso({comments: false})))
    .pipe(concat('obijudge.css'))
    .pipe(ifEnv.not('production', sourcemaps.write()))
    .pipe(gulp.dest('static/dist'));
})

gulp.task('fonts', function() {
    var google = gulp.src('static/src/fonts.list')
        .pipe(googleWebFonts({
            fontsDir: 'fonts',
            cssDir: './',
            cssFilename: 'fonts.css',
            format: 'woff',
        }))
        .pipe(gulp.dest('static/dist'));

    var katex = gulp.src('node_modules/katex/dist/fonts/*.woff*')
            .pipe(gulp.dest('static/dist/fonts'));

    return merge(google, katex);
});

gulp.task('images', function() {
    return gulp.src(['static/src/obi.ico', 'static/src/obi.svg'])
    .pipe(gulp.dest('static/dist'))
})

gulp.task('clean', function() {
    return gulp.src('static/dist').pipe(clean());
})

gulp.task('build', ['js', 'css', 'fonts', 'images']);

gulp.task('default', ['build']);
