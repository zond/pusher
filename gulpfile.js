var gulp = require('gulp');
var wrap = require('gulp-wrap-umd');

gulp.task('umd', function(){
  gulp.src(['js/pusher.js'])
  .pipe(gulp.dest('build/js/'));
});
