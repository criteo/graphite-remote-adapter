var gulp = require('gulp');
var rename = require("gulp-rename");

////////////////////////////
// BOOTSTRAP
////////////////////////////

// Take js from bootstrap and put it in static files
gulp.task('bootstrap_js', function() {
  return gulp.src('node_modules/bootstrap/dist/js/bootstrap.min.js')
    .pipe(gulp.dest('ui/static/js'));
});

// Take js from bootstrap and put it in static files
gulp.task('jquery', function() {
  return gulp.src('node_modules/jquery/dist/jquery.js')
    .pipe(gulp.dest('ui/static/js'));
});

// Take bootswatch theme and put it in static files
gulp.task('bootstrap_theme', function() {
  return gulp.src('node_modules/bootswatch/dist/lumen/bootstrap.css')
    .pipe(rename('./bootstrap-lumen.min.css'))
    .pipe(gulp.dest('ui/static/css/'));
});

gulp.task('bootstrap', gulp.series('bootstrap_js', 'jquery', 'bootstrap_theme'), () => {});

////////////////////////////
// FONT AWESOME
////////////////////////////

// Take CSS for fontawesome and put them in static files
gulp.task('fontawesome_css', function() {
  return gulp.src('node_modules/font-awesome/css/font-awesome.min.css')
    .pipe(gulp.dest('ui/static/css'));
});

// Take fonts from fontawesome and put them in static files
gulp.task('fontawesome_fonts', function() {
  return gulp.src("node_modules/font-awesome/fonts/*")
    .pipe(gulp.dest('ui/static/fonts'));
});

gulp.task('fontawesome', gulp.series('fontawesome_css','fontawesome_fonts'), () => {});

////////////////////////////
// DEFAULT
////////////////////////////

gulp.task('default', gulp.series('bootstrap', 'fontawesome'), () => {});