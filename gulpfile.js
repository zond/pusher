var gulp = require('gulp');

gulp.task('default', function(){
  gulp.src(['js/pusher.js']).pipe(gulp.dest('dist/'));
});
