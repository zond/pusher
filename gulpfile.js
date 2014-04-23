var gulp = require('gulp');
var wrap = require('gulp-wrap-umd');

gulp.task('umd', function(){
  gulp.src(['js/pusher.js'])
  .pipe(wrap({
    exports: 'Pusher'
  }))
  .pipe(gulp.dest('dist/'));
});
