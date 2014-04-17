var gulp = require('gulp');
var wrap = require('gulp-wrap-umd');

gulp.task('umd', function(){
  gulp.src(['js/pusher.js'])
  .pipe(wrap())
  .pipe(gulp.dest('dist/'));
});
