var gulp = require('gulp');
var wrap = require('gulp-wrap-umd');
var concat = require('gulp-concat');

gulp.task('umd', function(){
  gulp.src(['bower_components/eventEmitter/EventEmitter.js', 'js/pusher.js'])
  .pipe(concat('pusher.js'))
  .pipe(wrap({
    exports: 'Pusher',
    namespace: 'Pusher'
  }))
  .pipe(gulp.dest('dist/'));
});
