var gulp = require('gulp');
var rename = require("gulp-rename");

gulp.task('default', ['bootstrap', 'fontawesome'], () => {});


////////////////////////////
// BOOTSTRAP
////////////////////////////

gulp.task('bootstrap', ['bootstrap_theme'], () => {});

// bootstrap_js is not needed yet
// // Take js from bootstrap and put it in static files
// gulp.task('bootstrap_js', function() {
//   return gulp.src('node_modules/bootstrap/dist/js/bootstrap.min.js')
//     .pipe(gulp.dest('ui/static/js'));
// });

// Take bootswatch theme and put it in static files
gulp.task('bootstrap_theme', function() {
  return gulp.src('node_modules/bootswatch/dist/lumen/bootstrap.css')
    .pipe(rename('./bootstrap-lumen.min.css'))
    .pipe(gulp.dest('ui/static/css/'));
});


////////////////////////////
// FONT AWESOME
////////////////////////////

gulp.task('fontawesome', ['fontawesome_css','fontawesome_fonts'], () => {});


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
